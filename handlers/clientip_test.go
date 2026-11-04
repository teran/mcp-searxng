package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyClientIP(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		headers map[string]string
		wantIP  string
		wantOK  bool
	}{
		{name: "no gateway header", wantOK: false},
		{
			name:    "X-Real-IP",
			headers: map[string]string{"X-Real-IP": "203.0.113.9"},
			wantIP:  "203.0.113.9", wantOK: true,
		},
		{
			name:    "X-Forwarded-For chain uses leftmost",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.5, 10.0.0.2"},
			wantIP:  "203.0.113.5", wantOK: true,
		},
		{
			name:    "X-Real-IP takes precedence over X-Forwarded-For",
			headers: map[string]string{"X-Real-IP": "203.0.113.1", "X-Forwarded-For": "198.51.100.2"},
			wantIP:  "203.0.113.1", wantOK: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			ip, ok := proxyClientIP(req)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ip != tt.wantIP {
				t.Errorf("ip = %q, want %q", ip, tt.wantIP)
			}
		})
	}
}

func TestClientIPMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("injects gateway client IP into context", func(t *testing.T) {
		var got string
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got = ClientIPFrom(r.Context())
		})

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.4")

		ClientIPMiddleware(next).ServeHTTP(httptest.NewRecorder(), req)

		if got != "203.0.113.4" {
			t.Errorf("ClientIPFrom(ctx) = %q, want 203.0.113.4", got)
		}
	})

	t.Run("leaves context empty when no gateway header", func(t *testing.T) {
		var got string
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			got = ClientIPFrom(r.Context())
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		ClientIPMiddleware(next).ServeHTTP(rec, req)

		if got != "" {
			t.Errorf("ClientIPFrom(ctx) = %q, want empty", got)
		}
	})
}

// TestClientIPContextRoundTrip verifies the context helpers store and retrieve
// the client IP.
func TestClientIPContextRoundTrip(t *testing.T) {
	t.Parallel()

	if got := ClientIPFrom(context.Background()); got != "" {
		t.Errorf("ClientIPFrom on empty ctx = %q, want empty", got)
	}

	ctx := WithClientIP(context.Background(), "10.1.2.3")
	if got := ClientIPFrom(ctx); got != "10.1.2.3" {
		t.Errorf("ClientIPFrom(ctx) = %q, want 10.1.2.3", got)
	}
}
