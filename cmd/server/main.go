package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"resty.dev/v3"

	"github.com/teran/mcp-searxng/application"
	"github.com/teran/mcp-searxng/config"
	"github.com/teran/mcp-searxng/handlers"
	infra "github.com/teran/mcp-searxng/infrastructure/searxng"
	"github.com/teran/mcp-searxng/logging"
)

// Build-time variables injected by goreleaser (via ldflags).
var (
	version = "dev"
	commit  = "none"    //nolint:gochecknoglobals
	date    = "unknown" //nolint:gochecknoglobals
)

func main() {
	modeFlag := flag.String("mode", "", "launch mode: http|stdio (default stdio)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		// Logger is not configured yet — use logrus defaults.
		logrus.Fatalf("Failed to load configuration: %v", err)
	}

	// The -mode flag overrides MODE; re-validate since it can introduce an
	// invalid value that config.Load did not see.
	if *modeFlag != "" {
		cfg.Mode = *modeFlag
	}
	if err := cfg.Validate(); err != nil {
		logrus.Fatalf("Invalid configuration: %v", err)
	}

	logger, err := logging.New(logging.Options{
		Mode:     cfg.Mode,
		Level:    cfg.LogLevel,
		Format:   cfg.LogFormat,
		Filename: cfg.LogFilename,
	})
	if err != nil {
		// Logger failed to initialize (e.g. cannot open LOG_FILENAME) — use
		// logrus defaults since we have no usable logger.
		logrus.Fatalf("Failed to configure logger: %v", err)
	}

	if err := Run(*cfg, logger); err != nil {
		logger.Fatalf("Failed to start server: %v", err)
	}
}

// Run binds the listeners required by the configured mode and starts the
// server, waiting for a signal (SIGTERM/SIGINT) or server error to trigger
// graceful shutdown. It returns an error only if a listener cannot be bound
// (fail fast), or on successful shutdown returns nil.
func Run(cfg config.Config, logger *logrus.Logger) error {
	bindCtx := context.Background()

	metricsLn, err := bindObservability(bindCtx, cfg, logger)
	if err != nil {
		return err
	}
	defer func() { _ = metricsLn.Close() }()

	// Wire OS signals to a cancellable context. In production this is what
	// triggers graceful shutdown on SIGTERM/SIGINT; in tests a plain cancellable
	// context is used instead (no process-global signal races).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	switch cfg.Mode {
	case "stdio":
		return runStdio(ctx, cfg, logger, metricsLn, &mcp.StdioTransport{})
	default: // "http"
		mainLn, err := (&net.ListenConfig{}).Listen(bindCtx, "tcp", cfg.ListenAddr)
		if err != nil {
			return fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
		}
		defer func() { _ = mainLn.Close() }()
		return runHTTP(ctx, cfg, logger, mainLn, metricsLn)
	}
}

// bindObservability binds the internal observability listener (metrics +
// healthz) on cfg.InternalAddr. It is shared by both transports.
func bindObservability(ctx context.Context, cfg config.Config, logger *logrus.Logger) (net.Listener, error) {
	l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.InternalAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", cfg.InternalAddr, err)
	}
	logger.Infof("Internal observability listener bound on %s", handlers.SanitizeLog(l.Addr().String()))
	return l, nil
}

// serverDeps holds the shared dependencies constructed once and used by both
// the HTTP and stdio transports.
type serverDeps struct {
	srv          *mcp.Server
	promRegistry *prometheus.Registry
	metrics      *handlers.Metrics
	close        func()
}

// buildServer constructs the shared dependencies: the resty HTTP client, the
// SearXNG client, the search service, the MCP server, the Prometheus registry
// and metrics, and registers all tools (metrics + access-log wrapped). The
// source label distinguishes the transport for the L08 access log.
func buildServer(cfg config.Config, logger *logrus.Logger, source string) (*serverDeps, error) {
	// sharedRestyClient is reused across requests for connection pooling.
	// RedirectNoPolicy disables redirects to prevent credential forwarding (the
	// resty client never follows redirects), and the explicit 30s timeout bounds
	// each outbound call. Resty performs no retries by default, so outbound
	// errors are returned to the model rather than silently retried.
	sharedRestyClient := resty.NewWithTransportSettings(&resty.TransportSettings{
		MaxIdleConns:    10,
		IdleConnTimeout: 90 * time.Second,
	}).
		SetTimeout(30 * time.Second).
		SetRedirectPolicy(resty.RedirectNoPolicy())

	// Create the SearXNG client and search service (shared across all requests).
	searxngClient := infra.NewClient(cfg.SearXNGURL, sharedRestyClient)
	searchSvc := application.NewSearchService(searxngClient)

	// Create the MCP server instance.
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-searxng",
		Version: version,
	}, &mcp.ServerOptions{
		Logger: logging.Slog(logger),
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{ListChanged: false},
		},
	})

	// Create Prometheus registry and metrics collectors.
	promRegistry := prometheus.NewRegistry()
	metrics := handlers.NewMetrics(promRegistry)

	// Register tools via handler factories (service injected explicitly — no context lookup).
	handlers.RegisterToolsWithSource(srv, metrics, logger, searchSvc, source)

	return &serverDeps{
		srv:          srv,
		promRegistry: promRegistry,
		metrics:      metrics,
		close:        func() { _ = sharedRestyClient.Close() },
	}, nil
}

// healthzHandler reports service liveness. It bypasses all middleware.
func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// logStartupBanner writes the startup banner (L06) — the first log lines that
// identify the service, its version and its upstream. Shared by both transports.
func logStartupBanner(logger *logrus.Logger, cfg config.Config) {
	u, _ := url.Parse(cfg.SearXNGURL)
	logger.Infof("SearXNG URL: %s", handlers.SanitizeLog(u.Redacted()))
	logger.Infof("Version: %s, commit: %s, built: %s", version, commit, date)
}

// runStdio starts the MCP server on the provided transport (typically
// &mcp.StdioTransport{}) and the observability/metrics server on the provided
// pre-bound internal listener, then waits for ctx cancellation or a transport
// error. On ctx cancellation Run returns nil (treating ctx.Err() as expected
// termination), mirroring the HTTP path. The transport is injectable so tests
// can drive an in-memory round-trip.
func runStdio(ctx context.Context, cfg config.Config, logger *logrus.Logger, metricsLn net.Listener, transport mcp.Transport) error {
	deps, err := buildServer(cfg, logger, "stdio")
	if err != nil {
		return err
	}
	defer deps.close()

	// Startup banner first so it is the opening log line (L06).
	logStartupBanner(logger, cfg)

	// Internal observability server (metrics + healthz) — O01.
	metricsHandler := handlers.RegisterMetricsOnRegistry(deps.promRegistry)
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", metricsHandler)
	metricsMux.HandleFunc("GET /healthz", healthzHandler)

	metricsServer := &http.Server{
		Addr:              cfg.InternalAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Channel to capture server errors (buffered to hold one).
	errCh := make(chan error, 1)

	go func() {
		logger.Infof("Starting internal observability server on %s", handlers.SanitizeLog(metricsLn.Addr().String()))
		if err := metricsServer.Serve(metricsLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Run the MCP server on the transport until ctx is cancelled or the
	// session ends. In stdio there is no rate limiter and no HTTP middleware.
	runErr := deps.srv.Run(ctx, transport)

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Errorf("Internal server shutdown error: %v", err)
	}

	// A cancelled context is expected termination, not an error.
	if ctx.Err() != nil {
		logger.Info("Server stopped gracefully")
		return nil
	}
	return runErr
}

// runHTTP starts the MCP HTTP server and observability server on the provided
// pre-bound listeners and waits for ctx cancellation or a server error to
// trigger graceful shutdown. It returns nil on successful shutdown.
//
// Accepting pre-bound listeners (instead of binding by address) is what
// eliminates the free-port-then-rebind TOCTOU race in tests: the caller binds
// a listener exactly once and hands ownership to the server. Using ctx (rather
// than a raw OS signal) makes shutdown deterministic and testable.
func runHTTP(ctx context.Context, cfg config.Config, logger *logrus.Logger, mainLn, metricsLn net.Listener) error {
	deps, err := buildServer(cfg, logger, "http")
	if err != nil {
		return err
	}
	defer deps.close()

	srv := deps.srv
	promRegistry := deps.promRegistry
	metrics := deps.metrics

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
	}, logger)
	handler := handlers.RecoveryMiddleware(logger,
		handlers.MetricsMiddleware(metrics)(
			rateLimitMW(
				handlers.BodyLimitMiddleware(handlers.DefaultMaxRequestBodySize)(
					handlers.LoggingMiddleware(logger, mcpHandler),
				),
			),
		),
	)

	// Health-check endpoint — bypasses all middleware (auth, rate limit, etc.)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.Handle("/", handler)

	logStartupBanner(logger, cfg)

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
		Addr:              cfg.InternalAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Channel to capture server errors (buffered to hold both if both fail).
	errCh := make(chan error, 2)

	go func() {
		logger.Infof("Starting mcp-searxng server on %s", handlers.SanitizeLog(mainLn.Addr().String()))
		if err := mainServer.Serve(mainLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	go func() {
		logger.Infof("Starting Prometheus metrics server on %s", handlers.SanitizeLog(metricsLn.Addr().String()))
		if err := metricsServer.Serve(metricsLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Wait for ctx cancellation (shutdown signal) or a server error.
	select {
	case <-ctx.Done():
		logger.Info("Received shutdown signal, shutting down...")
	case err := <-errCh:
		logger.Errorf("Server error: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Stop the rate limiter background eviction goroutine.
	stopRateLimiter()

	// Shut down both servers in order.
	if err := mainServer.Shutdown(shutdownCtx); err != nil {
		logger.Errorf("Main server shutdown error: %v", err)
	}
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Errorf("Metrics server shutdown error: %v", err)
	}

	logger.Info("Server stopped gracefully")
	return nil
}
