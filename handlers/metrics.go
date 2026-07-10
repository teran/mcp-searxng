package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds Prometheus metric collectors for the MCP server.
type Metrics struct {
	toolRequestsTotal *prometheus.CounterVec
	toolDuration      *prometheus.HistogramVec
	activeRequests    prometheus.Gauge
}

// NewMetrics creates and registers all Prometheus metric collectors with the given registry.
// It registers:
//   - mcp_tool_requests_total   — CounterVec with labels {tool, status_class}
//   - mcp_tool_duration_seconds — HistogramVec with label {tool}
//   - mcp_active_requests       — Gauge (current in-flight requests)
func NewMetrics(reg *prometheus.Registry) *Metrics {
	m := &Metrics{
		toolRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "mcp_tool_requests_total",
				Help: "Total number of MCP tool requests partitioned by tool name and status class.",
			},
			[]string{"tool", "status_class"},
		),
		toolDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "mcp_tool_duration_seconds",
				Help:    "Histogram of MCP tool request durations in seconds, partitioned by tool name.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"tool"},
		),
		activeRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "mcp_active_requests",
				Help: "Current number of in-flight MCP requests.",
			},
		),
	}
	reg.MustRegister(m.toolRequestsTotal, m.toolDuration, m.activeRequests)
	return m
}

// WrapToolHandler wraps an MCP tool handler with Prometheus metrics recording.
// The toolName is the hardcoded name from tool registration — never from user
// input — ensuring label values are safe and bounded. Errors returned by the
// handler are classified as "4xx" status; successful calls as "2xx".
func WrapToolHandler[I, O any](metrics *Metrics, toolName string, handler mcp.ToolHandlerFor[I, O]) mcp.ToolHandlerFor[I, O] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input I) (*mcp.CallToolResult, O, error) {
		start := time.Now()

		result, output, err := handler(ctx, req, input)

		duration := time.Since(start).Seconds()
		statusClass := "2xx"
		if err != nil {
			statusClass = "4xx"
		}

		if metrics != nil {
			metrics.toolRequestsTotal.WithLabelValues(toolName, statusClass).Inc()
			metrics.toolDuration.WithLabelValues(toolName).Observe(duration)
		}

		return result, output, err
	}
}

// MetricsMiddleware returns an HTTP middleware that tracks the number of
// in-flight MCP requests via the mcp_active_requests gauge. It does not
// read the request body — per-tool metrics (counter + histogram) are
// recorded by WrapToolHandler at the handler registration level.
//
// activeRequests.Dec() is deferred to ensure it runs even if a downstream
// handler panics (caught by RecoveryMiddleware).
func MetricsMiddleware(metrics *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			metrics.activeRequests.Inc()
			defer metrics.activeRequests.Dec()
			next.ServeHTTP(w, r)
		})
	}
}

// RegisterMetricsOnRegistry registers the standard Go runtime and custom MCP
// metrics on the same registry, then returns an http.Handler for /metrics.
func RegisterMetricsOnRegistry(reg *prometheus.Registry) http.Handler {
	reg.MustRegister(collectors.NewGoCollector())
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}
