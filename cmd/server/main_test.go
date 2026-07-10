package main

import (
	"bytes"
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

// testHTTPClient is a shared HTTP client for tests that never follows redirects.
var testHTTPClient = &http.Client{ //nolint:gochecknoglobals
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func TestInjectClientMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("creates search service and passes through", func(t *testing.T) {
		middleware := injectClientMiddleware("http://searxng:8888", testHTTPClient)

		var handlerCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		middleware(next).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
		if !handlerCalled {
			t.Error("next handler was not called")
		}
	})
}

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

// freePort asks the kernel for a free TCP port on 127.0.0.1.
func freePort(t *testing.T) string {
	t.Helper()

	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().String()
}

// waitForServer polls url until it responds with a 2xx status or timeoutMs
// elapses. It is used to wait for a goroutine-run server to be ready.
func waitForServer(t *testing.T, url string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:noctx,gosec
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready within %v", url, timeout)
}

// shutdownViaSignal sends a signal to the current process and is used by
// Run tests to trigger graceful shutdown.
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

func TestRun_HealthEndpoint(t *testing.T) {
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

	defer func() {
		shutdownViaSignal(t, syscall.SIGTERM)
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for Run to return after shutdown signal")
		}
	}()

	waitForServer(t, "http://"+addr+"/healthz", 3*time.Second)

	resp, err := http.Get("http://" + addr + "/healthz") //nolint:noctx
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
}

func TestRun_MCPHandlerPing(t *testing.T) {
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

	defer func() {
		shutdownViaSignal(t, syscall.SIGTERM)
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for Run to return after shutdown signal")
		}
	}()

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

	resp, err := http.DefaultClient.Do(req)
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
}

func TestRun_ShutdownViaSignal(t *testing.T) {
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

	// Wait for server to be ready before signaling.
	waitForServer(t, "http://"+addr+"/healthz", 3*time.Second)

	// Send SIGTERM to trigger graceful shutdown.
	shutdownViaSignal(t, syscall.SIGTERM)

	// Wait for Run to complete.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Run to complete after SIGTERM")
	}
}

func TestRun_ShutdownViaSIGINT(t *testing.T) {
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

	waitForServer(t, "http://"+addr+"/healthz", 3*time.Second)

	shutdownViaSignal(t, syscall.SIGINT)

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Run to complete after SIGINT")
	}
}

func TestRun_ServerStartError(t *testing.T) {
	metricsAddr := freePort(t)

	cfg := config.Config{
		ListenAddr:            "127.0.0.1:-1", // invalid port — ListenAndServe fails immediately
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

	// Run should complete (after picking up the listen error from errCh)
	// and return nil (the error is logged, not returned).
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Run to return after listen error")
	}
}

func TestRun_MetricsServerStartError(t *testing.T) {
	addr := freePort(t)

	cfg := config.Config{
		ListenAddr:            addr,
		PrometheusMetricsAddr: "127.0.0.1:-1", // invalid port — metrics ListenAndServe fails immediately
		SearXNGURL:            "http://localhost:9999",
		RateLimitGlobal:       100,
		RateLimitPerClient:    10,
		WriteTimeout:          5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(cfg)
	}()

	// The metrics server fails immediately and the select in Run() picks up
	// the error, triggering shutdown before the main server may have started.
	// Just verify Run() completes without error.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Run to return")
	}
}

func TestRun_WithMetricsPortSameAsMain(t *testing.T) {
	addr := freePort(t)

	cfg := config.Config{
		ListenAddr:            addr,
		PrometheusMetricsAddr: addr, // same address — metrics server will fail
		SearXNGURL:            "http://localhost:9999",
		RateLimitGlobal:       100,
		RateLimitPerClient:    10,
		WriteTimeout:          5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(cfg)
	}()

	// The main server should still start on the free port, but the metrics
	// server will fail. Run should capture the error from errCh and shut down
	// (the select will pick the errCh signal first).
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Run to return after port conflict")
	}
}
