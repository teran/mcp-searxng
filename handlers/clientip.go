package handlers

import (
	"context"
	"net/http"
	"strings"
)

type clientIPKey struct{}

// WithClientIP returns a copy of ctx carrying the gateway-provided client IP
// used for the L08 access-log `source` field.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

// ClientIPFrom returns the client IP carried in ctx, or "" if none was set.
func ClientIPFrom(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)

	return ip
}

// proxyClientIP returns the client IP and true when a trusted reverse proxy
// (gateway) has populated X-Real-IP or X-Forwarded-For for the request;
// otherwise it returns "", false so the caller falls back to the transport
// label. X-Forwarded-For is a chain "client, proxy1, proxy2" — the leftmost
// (original client) entry is used.
func proxyClientIP(r *http.Request) (string, bool) {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip, true
	}

	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if idx := strings.IndexByte(fwd, ','); idx >= 0 {
			fwd = strings.TrimSpace(fwd[:idx])
		}

		return fwd, true
	}

	return "", false
}

// ClientIPMiddleware injects the gateway-provided client IP (X-Real-IP or
// X-Forwarded-For) into the request context so the L08 access log can record
// the real client as its `source`. When no gateway header is present the
// context is left empty and WrapAccessLog falls back to the transport label.
func ClientIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip, ok := proxyClientIP(r); ok {
			r = r.WithContext(WithClientIP(r.Context(), ip))
		}
		next.ServeHTTP(w, r)
	})
}
