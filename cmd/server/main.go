// Package main is the composition root of the mcp-searxng server: it selects
// the launch mode (stdio or Streamable HTTP), wires the layers and starts the
// listeners. Startup performs local wiring only — no outbound request is made
// (A02/N35).
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

// Named durations and sizes used at startup, kept as constants so the `mnd`
// linter has no bare magic numbers (C03GO) and the values are documented once.
const (
	restyMaxIdleConns   = 10
	restyIdleTimeout    = 90 * time.Second
	restyTimeout        = 30 * time.Second
	metricsReadHeaderTO = 10 * time.Second
	metricsWriteTimeout = 30 * time.Second
	metricsIdleTimeout  = 60 * time.Second
	mainReadHeaderTO    = 30 * time.Second
	mainReadTimeout     = 60 * time.Second
	mainIdleTimeout     = 120 * time.Second
	shutdownTimeout     = 30 * time.Second
	burstMultiplier     = 2
	serverErrBufferSize = 2
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
		MaxIdleConns:    restyMaxIdleConns,
		IdleConnTimeout: restyIdleTimeout,
	}).
		SetTimeout(restyTimeout).
		SetRedirectPolicy(resty.RedirectNoPolicy()).
		// Identify outbound requests (conformance N44/M10). The `version` var is
		// always non-empty (defaults to "dev"), so the header can never be
		// empty/malformed.
		SetHeader("User-Agent", fmt.Sprintf("mcp-searxng/%s (https://github.com/teran/mcp-searxng)", version))

	// Create the Prometheus registry first so the upstream metrics collector
	// can be attached to the SearXNG client (O03).
	promRegistry := prometheus.NewRegistry()

	// Create the SearXNG client and search service (shared across all requests).
	searxngClient := infra.NewClient(cfg.SearXNGURL, sharedRestyClient)
	searxngClient.SetMetrics(infra.NewUpstreamMetrics(promRegistry))
	searchSvc := application.NewSearchService(searxngClient)

	// Create the MCP server instance. Server-level Instructions (M08) and the
	// resources capability make the server self-describing over the protocol.
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-searxng",
		Version: version,
	}, &mcp.ServerOptions{
		Logger:       logging.Slog(logger),
		Instructions: handlers.ServerInstructions,
		Capabilities: &mcp.ServerCapabilities{
			Tools:     &mcp.ToolCapabilities{ListChanged: false},
			Resources: &mcp.ResourceCapabilities{},
		},
	})

	// Register the self-describing guidance resources (M09).
	handlers.AddGuidanceResources(srv)

	// Create MCP metrics collectors on the shared registry.
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
func runStdio(
	ctx context.Context, cfg config.Config, logger *logrus.Logger, metricsLn net.Listener, transport mcp.Transport,
) error {
	deps, err := buildServer(cfg, logger, "stdio")
	if err != nil {
		return err
	}
	defer deps.close()

	// Startup banner first so it is the opening log line (L06).
	logStartupBanner(logger, cfg)

	// Internal observability server (metrics + pprof + healthz) — O01.
	metricsHandler := handlers.ObservabilityHandler(deps.promRegistry, true)
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/", metricsHandler)

	metricsServer := &http.Server{
		Addr:              cfg.InternalAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: metricsReadHeaderTO,
		WriteTimeout:      metricsWriteTimeout,
		IdleTimeout:       metricsIdleTimeout,
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

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
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

	// Create the Streamable HTTP handler.
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server {
			return srv
		},
		&mcp.StreamableHTTPOptions{
			Stateless: true,
		},
	)

	handler, stopRateLimiter := buildHTTPHandler(logger, deps.metrics, cfg, mcpHandler)

	// Health-check endpoint — bypasses all middleware (auth, rate limit, etc.).
	// The MCP handler is wrapped with the standard net/http Prometheus metrics
	// (O02: latency / counter / response size).
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handlers.Healthz)
	mux.Handle("/", handlers.InstrumentServer(promRegistry, handler))

	logStartupBanner(logger, cfg)

	mainServer := newMainServer(cfg, mux)
	metricsServer := newMetricsServer(cfg.InternalAddr, handlers.ObservabilityHandler(promRegistry, false))

	// Channel to capture server errors (buffered to hold both if both fail).
	errCh := make(chan error, serverErrBufferSize)
	go serve(logger, errCh, "mcp-searxng server", mainLn, mainServer)
	go serve(logger, errCh, "Prometheus metrics server", metricsLn, metricsServer)

	// Wait for a shutdown signal or a server error, then stop the rate
	// limiter and gracefully shut both servers down in order.
	return awaitAndShutdown(ctx, logger, errCh, mainServer, metricsServer, stopRateLimiter)
}

// buildHTTPHandler constructs the Streamable HTTP handler wrapped in the
// middleware chain (recovery → metrics → rate limit → body limit → logging)
// and returns it together with the rate-limiter stop function.
func buildHTTPHandler(
	logger *logrus.Logger, metrics *handlers.Metrics, cfg config.Config, mcpHandler http.Handler,
) (http.Handler, func()) {
	rateLimitMW, stopRateLimiter := handlers.RateLimitMiddleware(handlers.RateLimiterConfig{
		GlobalLimit:    rate.Limit(cfg.RateLimitGlobal),
		GlobalBurst:    cfg.RateLimitGlobal * burstMultiplier,
		PerClientLimit: rate.Limit(cfg.RateLimitPerClient),
		PerClientBurst: cfg.RateLimitPerClient * burstMultiplier,
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

	return handler, stopRateLimiter
}

// newMainServer builds the main MCP Streamable HTTP server on the listen
// address, with the middleware chain as its handler.
func newMainServer(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: mainReadHeaderTO,
		ReadTimeout:       mainReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       mainIdleTimeout,
	}
}

// newMetricsServer builds the internal observability server on the given
// address serving the Prometheus metrics handler.
func newMetricsServer(addr string, handler http.Handler) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", handler)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: metricsReadHeaderTO,
		WriteTimeout:      metricsWriteTimeout,
		IdleTimeout:       metricsIdleTimeout,
	}
}

// serve runs an HTTP server on the pre-bound listener, forwarding any
// non-graceful error to errCh for the caller to act on.
func serve(logger *logrus.Logger, errCh chan<- error, name string, ln net.Listener, srv *http.Server) {
	logger.Infof("Starting %s on %s", name, handlers.SanitizeLog(ln.Addr().String()))
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errCh <- err
	}
}

// awaitAndShutdown blocks until ctx is cancelled or a server reports an error,
// then gracefully shuts down the main and metrics HTTP servers. It returns nil
// on successful shutdown.
func awaitAndShutdown(
	ctx context.Context,
	logger *logrus.Logger,
	errCh <-chan error,
	mainServer, metricsServer *http.Server,
	stopRateLimiter func(),
) error {
	// Wait for ctx cancellation (shutdown signal) or a server error.
	select {
	case <-ctx.Done():
		logger.Info("Received shutdown signal, shutting down...")
	case err := <-errCh:
		logger.Errorf("Server error: %v", err)
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
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
