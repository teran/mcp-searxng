package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"

	"github.com/teran/mcp-searxng/application"
	"github.com/teran/mcp-searxng/config"
	"github.com/teran/mcp-searxng/handlers"
	infra "github.com/teran/mcp-searxng/infrastructure/searxng"
)

// Build-time variables injected by goreleaser (via ldflags).
var (
	version = "dev"
	commit  = "none"    //nolint:gochecknoglobals
	date    = "unknown" //nolint:gochecknoglobals
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if err := Run(*cfg); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// Run creates and starts the MCP HTTP server, metrics server, and
// waits for a signal (SIGTERM/SIGINT) or server error to trigger
// graceful shutdown. It returns nil on successful shutdown or an
// error only if a non-recoverable failure occurs before the servers
// are started.
func Run(cfg config.Config) error {
	// sharedHTTPClient is reused across requests for connection pooling.
	// CheckRedirect is set to http.ErrUseLastResponse to prevent credential
	// forwarding — the http.Client never follows redirects.
	sharedHTTPClient := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			MaxIdleConns:       10,
			IdleConnTimeout:    90 * time.Second,
			DisableCompression: false,
			DisableKeepAlives:  false,
		},
	}

	// Create the MCP server instance.
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-searxng",
		Version: version,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{ListChanged: false},
		},
	})

	// Create Prometheus registry and metrics collectors.
	promRegistry := prometheus.NewRegistry()
	metrics := handlers.NewMetrics(promRegistry)

	// Register tools via handler factories.
	handlers.RegisterTools(srv, metrics)

	// Create the Streamable HTTP handler.
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server {
			return srv
		},
		&mcp.StreamableHTTPOptions{
			Stateless: true,
		},
	)

	// Wrap with middlewares (outermost to innermost):
	// recovery → metrics → rate limit → body limit → logging → client injection → MCP handler.
	rateLimitMW, stopRateLimiter := handlers.RateLimitMiddleware(handlers.RateLimiterConfig{
		GlobalLimit:    rate.Limit(cfg.RateLimitGlobal),
		GlobalBurst:    cfg.RateLimitGlobal * 2,
		PerClientLimit: rate.Limit(cfg.RateLimitPerClient),
		PerClientBurst: cfg.RateLimitPerClient * 2,
	})
	handler := handlers.RecoveryMiddleware(
		handlers.MetricsMiddleware(metrics)(
			rateLimitMW(
				handlers.BodyLimitMiddleware(handlers.DefaultMaxRequestBodySize)(
					handlers.LoggingMiddleware(
						injectClientMiddleware(cfg.SearXNGURL, sharedHTTPClient)(mcpHandler),
					),
				),
			),
		),
	)

	// Health-check endpoint — bypasses all middleware (auth, rate limit, etc.)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/", handler)

	u, _ := url.Parse(cfg.SearXNGURL)
	log.Printf("SearXNG URL: %s", handlers.SanitizeLog(u.Redacted()))
	log.Printf("Version: %s, commit: %s, built: %s", version, commit, date)

	// ---- Main MCP HTTP server ----
	mainServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       120 * time.Second,
	}

	// ---- Metrics HTTP server ----
	metricsHandler := handlers.RegisterMetricsOnRegistry(promRegistry)
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", metricsHandler)

	metricsServer := &http.Server{
		Addr:              cfg.PrometheusMetricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Channel to capture server errors (buffered to hold both if both fail).
	errCh := make(chan error, 2)

	go func() {
		log.Printf("Starting mcp-searxng server on %s", handlers.SanitizeLog(cfg.ListenAddr))
		if err := mainServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	go func() {
		log.Printf("Starting Prometheus metrics server on %s", handlers.SanitizeLog(cfg.PrometheusMetricsAddr))
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for SIGTERM or SIGINT for graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(quit)

	select {
	case sig := <-quit:
		log.Printf("Received signal %v, shutting down...", sig)
	case err := <-errCh:
		log.Printf("Server error: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop the rate limiter background eviction goroutine.
	stopRateLimiter()

	// Shut down both servers in order.
	if err := mainServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Main server shutdown error: %v", err)
	}
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Metrics server shutdown error: %v", err)
	}

	log.Println("Server stopped gracefully")
	return nil
}

// injectClientMiddleware creates the SearXNG client and attaches
// the search service to the context.
func injectClientMiddleware(searxngURL string, sharedHTTPClient *http.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			client := infra.NewClient(searxngURL, sharedHTTPClient)
			searchSvc := application.NewSearchService(client)

			ctx := r.Context()
			ctx = handlers.ContextWithServices(ctx, searchSvc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
