//go:build e2e

package main

// End-to-end test (C09GO) that runs a real SearXNG container via
// github.com/teran/go-docker-testsuite and drives the full MCP handler stack
// (mcp.NewServer + RegisterTools + StreamableHTTPHandler + middleware) exactly
// as production does through `run()`. It proves the real path
// tool-handler → application service → SearXNG HTTP client → live SearXNG
// container works end to end.
//
// This file carries `//go:build e2e` so it is excluded from the default unit
// run (`make test`). Run it with: `make e2e` (i.e. `go test -tags e2e ./...`).
// It requires a running Docker daemon; if Docker is unavailable the test skips
// rather than failing.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	dockerClient "github.com/docker/docker/client"
	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/wait"

	"github.com/teran/mcp-searxng/config"
)

// searxngImage is the container image used for the live SearXNG backend.
//
// Pinned to a concrete (non-":latest") tag: for non-latest images the
// go-docker-testsuite harness only checks local presence and never triggers a
// remote digest comparison, so a previously-pulled image is reused as-is (this
// avoids an unnecessary registry round-trip / re-pull on every run).
const searxngImage = "searxng/searxng:2026.9.23-3cd69d30e"

// searxngSettings is a minimal settings.yml that enables the JSON output format
// (required by the infrastructure client, which always requests format=json)
// and disables the limiter so the test client is not blocked as a bot.
const searxngSettings = `use_default_settings: true
server:
  secret_key: "e2e-test-secret-key-change-me"
  image_proxy: true
  limiter: false
search:
  formats:
    - html
    - json
`

// dockerAvailable reports whether a Docker daemon is reachable. When it is not
// (e.g. a CI runner without Docker), the e2e test skips instead of failing.
func dockerAvailable(ctx context.Context) bool {
	cli, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv, dockerClient.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	defer func() { _ = cli.Close() }()

	if _, err := cli.Ping(ctx); err != nil {
		return false
	}
	return true
}

// startSearXNG spins up a real SearXNG container configured for JSON output and
// waits for its worker to report ready (log-based readiness). It returns the
// base URL reachable from the test process. The container lifecycle is bound to
// the test.
func startSearXNG(t *testing.T, ctx context.Context) string {
	t.Helper()

	c, err := docker.NewContainerWithLifecycleT(
		t,
		"searxng",
		searxngImage,
		nil,
		docker.NewEnvironment(),
		docker.NewPortBindings().PortDNAT(docker.ProtoTCP, 8080),
		docker.WithFiles(
			docker.FileFromBytes("/etc/searxng/settings.yml", []byte(searxngSettings), 0o644, 0, 0),
		),
		docker.WithHostConfig(docker.WithMemoryLimit(512*1024*1024)),
	)
	if err != nil {
		t.Fatalf("failed to create SearXNG container: %v", err)
	}

	c.RunT(ctx)

	// Wait until SearXNG reports its worker is up. A log-based readiness probe
	// is used instead of ForHTTPGet because during startup granian can reset a
	// connection (EOF) before the worker is ready, which ForHTTPGet treats as a
	// fatal error rather than a transient retry.
	if err := wait.Wait(ctx, c,
		wait.ForLog(docker.NewSubstringMatcher("Started worker")),
		wait.WithTimeout(3*time.Minute),
	); err != nil {
		t.Fatalf("SearXNG container did not become ready: %v", err)
	}

	hp, err := c.URL(docker.ProtoTCP, 8080)
	if err != nil {
		t.Fatalf("failed to resolve SearXNG container URL: %v", err)
	}
	return "http://" + hp.String()
}

// TestE2E_SearchTool spins up a real SearXNG container, boots the full MCP
// server stack pointed at it, and issues a JSON-RPC tools/call for the `search`
// tool. It verifies that the real path tool-handler → SearXNG works: the SSE
// response carries no JSON-RPC error and the structuredContent echoes the query
// and contains the results array produced by the live SearXNG instance.
func TestE2E_SearchTool(t *testing.T) {
	// Route the SearXNG image through Docker Hub directly instead of any
	// IMAGE_PREFIX mirror configured for the host. The public SearXNG image is
	// used here verbatim; the pinned (non-":latest") tag also lets the harness
	// reuse an already-pulled image without a registry round-trip.
	t.Setenv("IMAGE_PREFIX", "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if !dockerAvailable(ctx) {
		t.Skip("Docker daemon is unavailable; skipping e2e test")
	}

	searxngURL := startSearXNG(t, ctx)

	mainLn := mustListen(t)
	metricsLn := mustListen(t)

	cfg := config.Config{
		ListenAddr:            mainLn.Addr().String(),
		PrometheusMetricsAddr: metricsLn.Addr().String(),
		SearXNGURL:            searxngURL,
		RateLimitGlobal:       100,
		RateLimitPerClient:    10,
		WriteTimeout:          5 * time.Second,
	}

	serverCtx, stop := context.WithCancel(context.Background())
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(serverCtx, cfg, mainLn, metricsLn)
	}()

	addr := mainLn.Addr().String()
	waitForServer(t, "http://"+addr+"/healthz", 30*time.Second)

	// JSON-RPC tools/call for the `search` tool.
	callReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "search",
			"arguments": map[string]any{
				"query": "test",
			},
		},
	}
	bodyBytes, err := json.Marshal(callReq)
	if err != nil {
		t.Fatalf("failed to marshal tools/call request: %v", err)
	}

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

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected status 200 or 202 for tools/call, got %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	// Parse the SSE data line.
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

	var msg map[string]any
	if err := json.Unmarshal([]byte(dataLine), &msg); err != nil {
		t.Fatalf("response data is not valid JSON: %v\ndata: %s", err, dataLine)
	}

	if msg["error"] != nil {
		t.Fatalf("unexpected JSON-RPC error in tools/call response: %v", msg["error"])
	}

	result, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected a result object in tools/call response, got %T: %v", msg["result"], msg["result"])
	}

	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("expected structuredContent object in result, got %T: %v", result["structuredContent"], result["structuredContent"])
	}

	// Query echo proves the response came from the real SearXNG instance.
	if got, want := structured["query"], "test"; got != want {
		t.Errorf("expected structuredContent.query %q, got %v", want, got)
	}

	// The results array proves the live search path returned actual data.
	results, ok := structured["results"].([]any)
	if !ok {
		t.Fatalf("expected structuredContent.results to be an array, got %T", structured["results"])
	}
	if len(results) == 0 {
		t.Error("expected at least one search result from the live SearXNG instance")
	} else if first, ok := results[0].(map[string]any); ok && first["url"] == "" {
		t.Error("expected the first search result to carry a url")
	}

	// Stop the server deterministically.
	stop()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("run returned unexpected error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for run to return after cancel")
	}
}
