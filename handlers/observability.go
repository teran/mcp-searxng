package handlers

import (
	"net/http"
	"net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// registerPprof registers the standard net/http/pprof handlers (O01) on mux.
func registerPprof(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

// Healthz reports service liveness. It bypasses all middleware.
func Healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// ObservabilityHandler builds the internal observability handler: Prometheus
// /metrics, the /debug/pprof/* endpoints and, optionally, the /healthz probe
// (O01). It is served on the internal address INTERNAL_ADDR in both launch
// modes, distinct from the MCP listen address (O04).
func ObservabilityHandler(reg *prometheus.Registry, includeHealthz bool) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", RegisterMetricsOnRegistry(reg))
	registerPprof(mux)
	if includeHealthz {
		mux.HandleFunc("GET /healthz", Healthz)
	}

	return mux
}

// InstrumentServer wraps h with the standard net/http Prometheus metrics (O02):
// per-request latency histogram, request counter and response-size histogram,
// registered on the given registry.
func InstrumentServer(reg *prometheus.Registry, h http.Handler) http.Handler {
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"code", "method"})
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"code", "method"})
	responseSize := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_response_size_bytes",
		Help:    "HTTP response size in bytes.",
		Buckets: prometheus.DefBuckets,
	}, []string{"code", "method"})
	reg.MustRegister(duration, requests, responseSize)

	return promhttp.InstrumentHandlerResponseSize(
		responseSize,
		promhttp.InstrumentHandlerCounter(
			requests,
			promhttp.InstrumentHandlerDuration(duration, h),
		),
	)
}
