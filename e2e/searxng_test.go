//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	docker "github.com/teran/go-docker-testsuite"
	"github.com/teran/go-docker-testsuite/wait"
	"resty.dev/v3"

	"github.com/teran/mcp-searxng/application"
	"github.com/teran/mcp-searxng/domain"
	infra "github.com/teran/mcp-searxng/infrastructure/searxng"
)

// searxngImage is the SearXNG container image used for the e2e test.
const searxngImage = "searxng/searxng:latest"

// searxngSettings enables the JSON search format that mcp-searxng requires and
// disables the limiter so the test is not throttled.
var searxngSettings = []byte(`server:
  secret_key: "e2e-test-only-secret"
  limiter: false
search:
  formats:
    - html
    - json
`)

// TestSearchEndToEnd spins up a real SearXNG instance in Docker and exercises
// the full path from the application service through the SearXNG HTTP client
// to the live backend (T02/C09GO).
func TestSearchEndToEnd(t *testing.T) {
	t.Parallel()

	c, err := docker.NewContainerWithLifecycleT(
		t,
		"searxng",
		searxngImage,
		nil,
		docker.NewEnvironment(),
		docker.NewPortBindings().PortDNAT(docker.ProtoTCP, 8080),
		docker.WithFiles(docker.FileFromBytes("/etc/searxng/settings.yml", searxngSettings, 0o644, 0, 0)),
	)
	if err != nil {
		t.Fatalf("create SearXNG container: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if err := c.Run(ctx); err != nil {
		t.Fatalf("run SearXNG container: %v", err)
	}

	// Wait until the SearXNG web UI is ready (HTTP 200). The go-docker-testsuite
	// ForHTTPGet strategy treats only ECONNREFUSED as transient and classifies
	// ECONNRESET ("connection reset by peer") as a fatal error — but SearXNG
	// resets connections while it is still booting, which intermittently flakes
	// CI. Wrap the strategy so any transient transport error is retried instead
	// of failing immediately; the whole probe is still bounded by the ctx
	// deadline (3 minutes) set above.
	resilient := func(ctx context.Context, t wait.Target) (bool, error) {
		ready, err := wait.ForHTTPGet(8080, wait.WithPath("/"), wait.WithResponseStatuses(200))(ctx, t)
		if err != nil {
			return false, nil // not ready yet — keep polling
		}
		return ready, nil
	}
	if err := wait.Wait(ctx, c, resilient); err != nil {
		t.Fatalf("wait for SearXNG readiness: %v", err)
	}

	hp, err := c.URL(docker.ProtoTCP, 8080)
	if err != nil {
		t.Fatalf("resolve SearXNG URL: %v", err)
	}
	baseURL := "http://" + hp.String()

	client := infra.NewClient(baseURL, resty.New().SetTimeout(30*time.Second))
	svc := application.NewSearchService(client)

	// A broad query that any configured engine should answer; assert the full
	// round-trip succeeds and echoes the query back.
	res, err := svc.Search(ctx, domain.SearchParams{Query: "mcp"})
	if err != nil {
		t.Fatalf("Search against live SearXNG failed: %v", err)
	}
	if res == nil {
		t.Fatal("Search returned nil response")
	}
	if res.Query != "mcp" {
		t.Errorf("Search.Query = %q, want %q", res.Query, "mcp")
	}
}
