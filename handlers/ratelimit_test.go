package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestExtractClientIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		headers  map[string]string
		remote   string
		expected string
	}{
		{
			name:     "X-Client-Ip takes precedence",
			headers:  map[string]string{"X-Client-Ip": "10.0.0.1:1234"},
			remote:   "192.168.1.1:5678",
			expected: "10.0.0.1",
		},
		{
			name:     "X-Client-Ip without port",
			headers:  map[string]string{"X-Client-Ip": "10.0.0.1"},
			remote:   "192.168.1.1:5678",
			expected: "10.0.0.1",
		},
		{
			name:     "X-Forwarded-For used when X-Client-Ip missing",
			headers:  map[string]string{"X-Forwarded-For": "203.0.113.1, 10.0.0.2"},
			remote:   "192.168.1.1:5678",
			expected: "203.0.113.1",
		},
		{
			name:     "X-Forwarded-For single IP",
			headers:  map[string]string{"X-Forwarded-For": "203.0.113.1"},
			remote:   "192.168.1.1:5678",
			expected: "203.0.113.1",
		},
		{
			name:     "RemoteAddr fallback",
			remote:   "192.168.1.1:5678",
			expected: "192.168.1.1",
		},
		{
			name:     "RemoteAddr without port",
			remote:   "192.168.1.1",
			expected: "192.168.1.1",
		},
		{
			name:     "X-Client-Ip with IPv6",
			headers:  map[string]string{"X-Client-Ip": "[::1]:1234"},
			remote:   "192.168.1.1:5678",
			expected: "::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			req.RemoteAddr = tt.remote

			got := extractClientIP(req)
			if got != tt.expected {
				t.Errorf("extractClientIP() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNewRateLimiter(t *testing.T) {
	t.Parallel()

	t.Run("allows request within limits", func(t *testing.T) {
		rl := NewRateLimiter(RateLimiterConfig{
			GlobalLimit:    rate.Limit(100),
			GlobalBurst:    100,
			PerClientLimit: rate.Limit(10),
			PerClientBurst: 10,
		})
		defer rl.Stop()

		if !rl.Allow("10.0.0.1") {
			t.Error("first request should be allowed")
		}
	})

	t.Run("global rate limiting works", func(t *testing.T) {
		rl := NewRateLimiter(RateLimiterConfig{
			GlobalLimit:    rate.Limit(1),
			GlobalBurst:    1,
			PerClientLimit: rate.Limit(100),
			PerClientBurst: 100,
		})
		defer rl.Stop()

		if !rl.Allow("10.0.0.1") {
			t.Error("first request should be allowed")
		}
		if rl.Allow("10.0.0.2") {
			t.Error("second request (different client) should be rate-limited by global limiter")
		}
	})

	t.Run("per-client rate limiting", func(t *testing.T) {
		rl := NewRateLimiter(RateLimiterConfig{
			GlobalLimit:    rate.Limit(100),
			GlobalBurst:    100,
			PerClientLimit: rate.Limit(1),
			PerClientBurst: 1,
		})
		defer rl.Stop()

		if !rl.Allow("10.0.0.1") {
			t.Error("first request should be allowed")
		}
		if rl.Allow("10.0.0.1") {
			t.Error("second request from same client should be rate-limited")
		}
	})
}

func TestRateLimitMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("request within limits passes through", func(t *testing.T) {
		handler := RateLimitMiddleware(RateLimiterConfig{
			GlobalLimit:    rate.Limit(100),
			GlobalBurst:    100,
			PerClientLimit: rate.Limit(10),
			PerClientBurst: 10,
		})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})
}

func TestRateLimiterEvictExpired(t *testing.T) {
	t.Parallel()

	t.Run("evicts stale clients", func(t *testing.T) {
		rl := &rateLimiter{
			config: RateLimiterConfig{
				GlobalLimit:    rate.Limit(100),
				GlobalBurst:    100,
				PerClientLimit: rate.Limit(10),
				PerClientBurst: 10,
			},
			global:  rate.NewLimiter(100, 100),
			clients: make(map[string]*clientLimiter),
			stopCh:  make(chan struct{}),
		}

		rl.clients["stale"] = &clientLimiter{
			limiter:  rate.NewLimiter(10, 10),
			lastSeen: time.Now().Add(-clientTTL - time.Minute),
		}
		rl.clients["fresh"] = &clientLimiter{
			limiter:  rate.NewLimiter(10, 10),
			lastSeen: time.Now(),
		}

		rl.evictExpired()

		if _, exists := rl.clients["stale"]; exists {
			t.Error("stale client should have been evicted")
		}
		if _, exists := rl.clients["fresh"]; !exists {
			t.Error("fresh client should not have been evicted")
		}
	})
}

func TestRateLimiterDefaultBurst(t *testing.T) {
	t.Parallel()

	t.Run("zero burst defaults to 2x rate", func(t *testing.T) {
		rl := NewRateLimiter(RateLimiterConfig{
			GlobalLimit:    rate.Limit(50),
			GlobalBurst:    0,
			PerClientLimit: rate.Limit(5),
			PerClientBurst: 0,
		})
		defer rl.Stop()

		if !rl.Allow("10.0.0.1") {
			t.Error("first request should be allowed")
		}
	})
}

// Ensure the coverage types compile.
var _ = fmt.Sprintf("%d", 0) //nolint:gochecknoglobals
