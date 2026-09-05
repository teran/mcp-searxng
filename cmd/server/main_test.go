package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/teran/mcp-searxng/config"
)

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	if rr.Body.String() != `{"status":"ok"}` {
		t.Errorf("expected body {\"status\":\"ok\"}, got %q", rr.Body.String())
	}
}

// mustListen binds a listener on 127.0.0.1:0 exactly once and returns it. The
// listener is owned by the server (via run) which closes it. Binding once
// avoids the free-port-then-rebind TOCTOU race that caused intermittent
// "address already in use" flakes (and unstable coverage) under parallel runs.
func mustListen(t *testing.T) net.Listener {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to bind listener: %v", err)
	}
	return l
}

// freePort returns a free TCP address on 127.0.0.1. It is used only where the
// assertion tolerates a port conflict (the Run bind-error tests), so the small
// free-then-rebind window is harmless there.
func freePort(t *testing.T) string {
	t.Helper()

	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().String()
}

// testClient performs HTTP requests with keep-alives disabled. This ensures
// each server-side connection is closed right after the response, so
// http.Server.Shutdown does not block waiting for a pooled keep-alive
// connection to return to idle.
var testClient = &http.Client{
	Transport: &http.Transport{DisableKeepAlives: true},
}

// waitForServer polls url until it responds with a 2xx status or timeoutMs
// elapses. It is used to wait for a goroutine-run server to be ready.
func waitForServer(t *testing.T, url string, timeout time.Duration) {
	t.Helper()

	if !serverReady(url, timeout) {
		t.Fatalf("server at %s did not become ready within %v", url, timeout)
	}
}

// serverReady polls url until it responds successfully or timeout elapses,
// returning whether it became ready. Unlike waitForServer it never fails the
// test, so callers can retry on transient startup conflicts.
func serverReady(url string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := testClient.Get(url) //nolint:noctx,gosec
		if err == nil {
			_ = resp.Body.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// shutdownViaSignal sends a signal to the current process. It is used only by
// TestRun_Success to stop a server started through the exported Run (which
// wires shutdown to an OS signal via signal.NotifyContext).
func shutdownViaSignal(t *testing.T, sig os.Signal) {
	t.Helper()

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("failed to find self process: %v", err)
	}
	if err := p.Signal(sig); err != nil {
		t.Fatalf("failed to send signal: %v", err)
	}
}

func newTestConfig(mainLn, metricsLn net.Listener) config.Config {
	return config.Config{
		ListenAddr:            mainLn.Addr().String(),
		PrometheusMetricsAddr: metricsLn.Addr().String(),
		SearXNGURL:            "http://localhost:9999",
		RateLimitGlobal:       100,
		RateLimitPerClient:    10,
		WriteTimeout:          5 * time.Second,
	}
}

func TestRun_HealthEndpoint(t *testing.T) {
	mainLn := mustListen(t)
	metricsLn := mustListen(t)

	cfg := newTestConfig(mainLn, metricsLn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cfg, mainLn, metricsLn)
	}()

	waitForServer(t, "http://"+mainLn.Addr().String()+"/healthz", 3*time.Second)

	resp, err := testClient.Get("http://" + mainLn.Addr().String() + "/healthz") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if string(bytes.TrimSpace(body)) != `{"status":"ok"}` {
		t.Errorf("expected body {\"status\":\"ok\"}, got %q", string(body))
	}

	// Stop the server deterministically via context cancellation.
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("run returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for run to return after cancel")
	}
}

func TestRun_MCPHandlerPing(t *testing.T) {
	mainLn := mustListen(t)
	metricsLn := mustListen(t)

	cfg := newTestConfig(mainLn, metricsLn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cfg, mainLn, metricsLn)
	}()

	addr := mainLn.Addr().String()
	waitForServer(t, "http://"+addr+"/", 3*time.Second)

	// JSON-RPC ping request: {"jsonrpc":"2.0","id":1,"method":"ping"}
	pingReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "ping",
	}
	bodyBytes, err := json.Marshal(pingReq)
	if err != nil {
		t.Fatalf("failed to marshal ping request: %v", err)
	}

	// The MCP Streamable HTTP handler requires both content types in Accept.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr+"/", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	resp, err := testClient.Do(req)
	if err != nil {
		t.Fatalf("POST /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Streamable HTTP handler may respond with either 200 (JSON-RPC response)
	// or 202 (pending stream session). Both are valid responses for a ping.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected status 200 or 202 for ping, got %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if len(respBody) == 0 {
		t.Fatal("empty response body")
	}

	// The Streamable HTTP handler returns Server-Sent Events format:
	//   event: message
	//   data: {"jsonrpc":"2.0","id":1,"result":{}}
	//
	// Extract the data line and parse it as JSON.
	var dataLine string
	for _, line := range bytes.Split(respBody, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("data: ")) {
			dataLine = string(bytes.TrimPrefix(trimmed, []byte("data: ")))
			break
		}
	}

	if dataLine == "" {
		t.Fatalf("no SSE data line found in response:\n%s", string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(dataLine), &result); err != nil {
		t.Fatalf("response data is not valid JSON: %v\ndata: %s\nraw: %s", err, dataLine, string(respBody))
	}

	if result["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc '2.0', got %v", result["jsonrpc"])
	}

	// For ping, the result should be present (empty object) or no error.
	if result["error"] != nil {
		t.Errorf("unexpected error in ping response: %v", result["error"])
	}

	// Stop the server deterministically via context cancellation.
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("run returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for run to return after cancel")
	}
}

func TestRun_ShutdownViaContext(t *testing.T) {
	mainLn := mustListen(t)
	metricsLn := mustListen(t)

	cfg := newTestConfig(mainLn, metricsLn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cfg, mainLn, metricsLn)
	}()

	// Wait for server to be ready before cancelling.
	waitForServer(t, "http://"+mainLn.Addr().String()+"/healthz", 3*time.Second)

	// Cancel the context to trigger graceful shutdown.
	cancel()

	// Wait for run to complete.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("run returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for run to complete after cancel")
	}
}

func TestRun_ShutdownBeforeReady(t *testing.T) {
	mainLn := mustListen(t)
	metricsLn := mustListen(t)

	cfg := newTestConfig(mainLn, metricsLn)

	// Cancel immediately, before the servers are up — run must still return nil.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cfg, mainLn, metricsLn)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("run returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for run to complete after pre-cancel")
	}
}

func TestRun_ServerStartError(t *testing.T) {
	// Both addresses are invalid — the main listener bind fails first and Run
	// returns a bind error (fail fast, no goroutine needed).
	cfg := config.Config{
		ListenAddr:            "127.0.0.1:-1", // invalid port — net.Listen fails immediately
		PrometheusMetricsAddr: "127.0.0.1:-1",
		SearXNGURL:            "http://localhost:9999",
		RateLimitGlobal:       100,
		RateLimitPerClient:    10,
		WriteTimeout:          5 * time.Second,
	}

	if err := Run(cfg); err == nil {
		t.Fatal("expected Run to return an error for an invalid listen address")
	}
}

func TestRun_MetricsServerStartError(t *testing.T) {
	cfg := config.Config{
		ListenAddr:            freePort(t),    // valid — main bind succeeds
		PrometheusMetricsAddr: "127.0.0.1:-1", // invalid — metrics bind fails
		SearXNGURL:            "http://localhost:9999",
		RateLimitGlobal:       100,
		RateLimitPerClient:    10,
		WriteTimeout:          5 * time.Second,
	}

	// Whether the main port is grabbed (TOCTOU) or the metrics bind fails, Run
	// must return a bind error — so this assertion is robust either way.
	if err := Run(cfg); err == nil {
		t.Fatal("expected Run to return an error for an invalid metrics address")
	}
}

func TestRun_WithMetricsPortSameAsMain(t *testing.T) {
	addr := freePort(t)

	cfg := config.Config{
		ListenAddr:            addr,
		PrometheusMetricsAddr: addr, // same address — metrics bind fails
		SearXNGURL:            "http://localhost:9999",
		RateLimitGlobal:       100,
		RateLimitPerClient:    10,
		WriteTimeout:          5 * time.Second,
	}

	if err := Run(cfg); err == nil {
		t.Fatal("expected Run to return an error on metrics/main address conflict")
	}
}

func TestRun_Success(t *testing.T) {
	// Exercises the exported Run success path: both listeners bind, the signal
	// context is created, and run serves until a SIGTERM triggers shutdown.
	// Retries on transient ephemeral-port conflicts (the small free-port-then-
	// rebind window is harmless here because the assertion tolerates a retry).
	for attempt := 0; attempt < 5; attempt++ {
		addr := freePort(t)
		metricsAddr := freePort(t)

		cfg := config.Config{
			ListenAddr:            addr,
			PrometheusMetricsAddr: metricsAddr,
			SearXNGURL:            "http://localhost:9999",
			RateLimitGlobal:       100,
			RateLimitPerClient:    10,
			WriteTimeout:          5 * time.Second,
		}

		errCh := make(chan error, 1)
		go func() {
			errCh <- Run(cfg)
		}()

		// Poll until the server is up. This also guarantees Run has registered
		// its signal handler, so the SIGTERM below is reliably caught.
		if !serverReady("http://"+addr+"/healthz", 3*time.Second) {
			// Server never came up — likely a transient ephemeral-port conflict
			// that made Run return a bind error. Drain and retry.
			select {
			case <-errCh:
			default:
			}
			continue
		}

		shutdownViaSignal(t, syscall.SIGTERM)

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("Run returned unexpected error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for Run to return after SIGTERM")
		}
		return // success
	}
	t.Fatal("Run success path not reproducible after 5 attempts")
}
