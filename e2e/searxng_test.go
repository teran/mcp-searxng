//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	docker "github.com/teran/go-docker-testsuite"
	"resty.dev/v3"

	"github.com/teran/mcp-searxng/application"
	"github.com/teran/mcp-searxng/domain"
	infra "github.com/teran/mcp-searxng/infrastructure/searxng"
)

// searxngImage is the SearXNG container image used for the e2e test.
const searxngImage = "searxng/searxng:latest"

// searxngSettings enables the JSON search format that mcp-searxng requires,
// disables the limiter so the test is not throttled, and (via
// use_default_settings: true) keeps the stock SearXNG defaults — including
// search.default_doi_resolver — which every request reads. Without that flag the
// injected file REPLACES the whole default settings.yml and requests fail with
// KeyError: 'default_doi_resolver' -> HTTP 500.
var searxngSettings = []byte(`use_default_settings: true
server:
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

	// Wait until the SearXNG web UI is ready (HTTP 200). This explicit poll
	// replaces the harness wait.ForHTTPGet, which classifies ECONNRESET
	// ("connection reset by peer") as a fatal error and, once wrapped for retry,
	// hides the true cause behind an opaque "context deadline exceeded". Here we
	// poll ourselves, remember the last probe error, and on timeout dump the
	// SearXNG container logs — so a genuine boot/config failure (e.g. a rejected
	// settings.yml) is diagnosable from the CI log instead of an unhelpful
	// timeout. The whole probe is bounded by the ctx deadline (3 minutes).
	lastErr := fmt.Errorf("readiness probe never ran")
readyLoop:
	for {
		hp, urlErr := c.URL(docker.ProtoTCP, 8080)
		if urlErr != nil {
			lastErr = fmt.Errorf("resolve URL: %w", urlErr)
		} else {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+hp.String()+"/", nil)
			resp, reqErr := http.DefaultClient.Do(req)
			if reqErr != nil {
				lastErr = fmt.Errorf("GET /: %w", reqErr)
			} else {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					break readyLoop
				}
				lastErr = fmt.Errorf("GET / returned status %d", resp.StatusCode)
			}
		}

		select {
		case <-ctx.Done():
			logCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			logs, _ := c.GetOutput(logCtx, func(string) bool { return true })
			t.Fatalf("SearXNG not ready before deadline; last probe error: %v\n--- SearXNG container logs ---\n%s",
				lastErr, strings.Join(logs, "\n"))
		case <-time.After(500 * time.Millisecond):
		}
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
