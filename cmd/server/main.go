package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
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

// Run binds the main and metrics listeners from cfg and starts the MCP HTTP
// server, metrics server, waiting for a signal (SIGTERM/SIGINT) or server
// error to trigger graceful shutdown. It returns an error only if a listener
// cannot be bound (fail fast), or on successful shutdown returns nil.
func Run(cfg config.Config) error {
	mainLn, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}
	defer func() { _ = mainLn.Close() }()

	metricsLn, err := net.Listen("tcp", cfg.PrometheusMetricsAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.PrometheusMetricsAddr, err)
	}
	defer func() { _ = metricsLn.Close() }()

	// Wire OS signals to a cancellable context. In production this is what
	// triggers graceful shutdown on SIGTERM/SIGINT; in tests a plain cancellable
	// context is used instead (no process-global signal races).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	return run(ctx, cfg, mainLn, metricsLn)
}

// run starts the MCP HTTP server and metrics server on the provided
// pre-bound listeners and waits for ctx cancellation or a server error to
// trigger graceful shutdown. It returns nil on successful shutdown.
//
// Accepting pre-bound listeners (instead of binding by address) is what
// eliminates the free-port-then-rebind TOCTOU race in tests: the caller binds
// a listener exactly once and hands ownership to the server. Using ctx (rather
// than a raw OS signal) makes shutdown deterministic and testable.
func run(ctx context.Context, cfg config.Config, mainLn, metricsLn net.Listener) error {
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

	// Create the SearXNG client and search service (shared across all requests).
	searxngClient := infra.NewClient(cfg.SearXNGURL, sharedHTTPClient)
	searchSvc := application.NewSearchService(searxngClient)

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

	// Register tools via handler factories (service injected explicitly — no context lookup).
	handlers.RegisterTools(srv, metrics, searchSvc)

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
	// recovery → metrics → rate limit → body limit → logging → MCP handler.
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
					handlers.LoggingMiddleware(mcpHandler),
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
		log.Printf("Starting mcp-searxng server on %s", handlers.SanitizeLog(mainLn.Addr().String()))
		if err := mainServer.Serve(mainLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	go func() {
		log.Printf("Starting Prometheus metrics server on %s", handlers.SanitizeLog(metricsLn.Addr().String()))
		if err := metricsServer.Serve(metricsLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for ctx cancellation (shutdown signal) or a server error.
	select {
	case <-ctx.Done():
		log.Printf("Received shutdown signal, shutting down...")
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
