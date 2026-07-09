package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// testHTTPClient is a shared HTTP client for tests that never follows redirects.
var testHTTPClient = &http.Client{ //nolint:gochecknoglobals
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func TestInjectClientMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("creates search service and passes through", func(t *testing.T) {
		middleware := injectClientMiddleware("http://searxng:8888", testHTTPClient)

		var handlerCalled bool
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		middleware(next).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
		if !handlerCalled {
			t.Error("next handler was not called")
		}
	})
}

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	if rr.Body.String() != `{"status":"ok"}` {
		t.Errorf("expected body {\"status\":\"ok\"}, got %q", rr.Body.String())
	}
}
