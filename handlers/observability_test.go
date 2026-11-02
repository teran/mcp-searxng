package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TestObservabilityHandler verifies O01: the internal observability handler
// serves /metrics and /debug/pprof/*, plus /healthz when requested, and returns
// 404 for unknown paths.
func TestObservabilityHandler(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	h := ObservabilityHandler(reg, true)

	cases := []struct {
		path string
		want int
	}{
		{"/metrics", http.StatusOK},
		{"/debug/pprof/", http.StatusOK},
		{"/debug/pprof/cmdline", http.StatusOK},
		{"/healthz", http.StatusOK},
		{"/unknown", http.StatusNotFound},
	}

	for _, testCase := range cases {
		t.Run(testCase.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, testCase.path, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != testCase.want {
				t.Errorf("%s = %d, want %d", testCase.path, rec.Code, testCase.want)
			}
		})
	}
}

// TestObservabilityHandlerNoHealthz verifies /healthz is omitted when requested.
func TestObservabilityHandlerNoHealthz(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	h := ObservabilityHandler(reg, false)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/healthz = %d, want 404 (healthz not enabled)", rec.Code)
	}
}

// TestInstrumentServer verifies O02: wrapping a handler with InstrumentServer
// exposes the standard net/http Prometheus metrics (latency / counter /
// response size) after a request.
func TestInstrumentServer(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	h := InstrumentServer(reg, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handler = %d, want 200", rec.Code)
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	names := make(map[string]bool, len(families))
	for _, mf := range families {
		names[mf.GetName()] = true
	}

	for _, want := range []string{
		"http_requests_total",
		"http_request_duration_seconds",
		"http_response_size_bytes",
	} {
		if !names[want] {
			t.Errorf("missing standard net/http metric %q after request", want)
		}
	}
}
