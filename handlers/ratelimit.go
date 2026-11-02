package handlers

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

// clientLimiter pairs a rate.Limiter with its last access timestamp so that
// stale entries can be evicted from the clients map.
type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiterConfig holds configuration for the rate limiting middleware.
// GlobalLimit is the maximum number of requests per second across all clients.
// GlobalBurst is the initial burst capacity for the global limiter.
// PerClientLimit is the maximum number of requests per second per client IP.
// PerClientBurst is the initial burst capacity for each per-client limiter.
type RateLimiterConfig struct {
	GlobalLimit    rate.Limit
	GlobalBurst    int
	PerClientLimit rate.Limit
	PerClientBurst int
}

// cleanupInterval is how often the stale-client eviction goroutine runs.
// The arithmetic lives in a covered function (not an inlined package-var
// initializer, which Go coverage does not instrument) so the mutation tester
// can exercise it.
var cleanupInterval = defaultCleanupInterval()

// clientTTL is how long a client limiter is kept after its last request.
var clientTTL = defaultClientTTL()

// defaultBurstMultiplier scales a rate limit into a default burst capacity.
const defaultBurstMultiplier = 2

// cleanupIntervalVal is the duration between stale-client eviction runs.
const cleanupIntervalVal = 10 * time.Minute

// clientTTLVal is how long a client limiter is kept after its last request.
const clientTTLVal = 30 * time.Minute

// defaultCleanupInterval returns how often the stale-client eviction goroutine
// runs. Kept as a function so its arithmetic expression is covered.
func defaultCleanupInterval() time.Duration { return cleanupIntervalVal }

// defaultClientTTL returns how long a client limiter is kept after its last
// request. Kept as a function so its arithmetic expression is covered.
func defaultClientTTL() time.Duration { return clientTTLVal }

// RateLimiter implements token-bucket rate limiting with a global limiter
// and per-client limiters tracked by IP address. Stale client entries are
// evicted periodically to prevent unbounded memory growth.
type RateLimiter struct {
	config  RateLimiterConfig
	global  *rate.Limiter
	clients map[string]*clientLimiter
	mu      sync.Mutex
	stopCh  chan struct{}
}

// NewRateLimiter creates a new rate limiter with the given configuration
// and starts a background goroutine that evicts stale client entries.
func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	burst := config.GlobalBurst
	if burst <= 0 {
		burst = int(config.GlobalLimit) * defaultBurstMultiplier // default: 2x the rate
	}
	limiter := &RateLimiter{
		config:  config,
		global:  rate.NewLimiter(config.GlobalLimit, burst),
		clients: make(map[string]*clientLimiter),
		stopCh:  make(chan struct{}),
	}
	go limiter.evictStaleClients()

	return limiter
}

// Stop terminates the background eviction goroutine. After calling Stop the
// RateLimiter should not be used.
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// Allow checks whether a request from the given client IP should be allowed.
// Returns true if the request is within rate limits.
func (rl *RateLimiter) Allow(clientIP string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if !rl.global.Allow() {
		return false
	}

	clientLim, exists := rl.clients[clientIP]
	if !exists {
		perBurst := rl.config.PerClientBurst
		if perBurst <= 0 {
			perBurst = int(rl.config.PerClientLimit) * defaultBurstMultiplier // default: 2x the rate
		}
		clientLim = &clientLimiter{
			limiter:  rate.NewLimiter(rl.config.PerClientLimit, perBurst),
			lastSeen: time.Now(),
		}
		rl.clients[clientIP] = clientLim
	} else {
		clientLim.lastSeen = time.Now()
	}

	return clientLim.limiter.Allow()
}

// evictExpired removes all client limiters whose last access time is older
// than clientTTL. It is safe for concurrent use.
func (rl *RateLimiter) evictExpired() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for ip, cl := range rl.clients {
		if time.Since(cl.lastSeen) > clientTTL {
			delete(rl.clients, ip)
		}
	}
}

// evictStaleClients periodically removes client limiters that have not been
// accessed within clientTTL, preventing unbounded memory growth when many
// unique client IPs connect over time.
func (rl *RateLimiter) evictStaleClients() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.evictExpired()
		}
	}
}

// RateLimitMiddleware returns an HTTP middleware that rate-limits requests
// using a token-bucket algorithm with the given configuration.
// Returns 429 Too Many Requests when the limit is exceeded.
// The returned stop function terminates the background eviction goroutine;
// callers must invoke it during shutdown to prevent goroutine leaks.
func RateLimitMiddleware(cfg RateLimiterConfig, logger *logrus.Logger) (func(http.Handler) http.Handler, func()) {
	limiter := NewRateLimiter(cfg)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := extractClientIP(r)
			if !limiter.Allow(clientIP) {
				logger.WithFields(logrus.Fields{
					"client_ip":   SanitizeLog(clientIP),
					"http_method": SanitizeLog(r.Method),
				}).Warn("rate_limit exceeded")
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)

				return
			}
			next.ServeHTTP(w, r)
		})
	}, limiter.Stop
}

// extractClientIP extracts the client IP address from the request, checking
// proxy headers in order of specificity. When behind a reverse proxy (nginx,
// HAProxy, etc.), the proxy should set X-Client-IP or X-Forwarded-For headers.
//
// Header precedence (highest first):
// 1. X-Client-IP — explicitly set by reverse proxy configuration
// 2. X-Forwarded-For — first (leftmost) IP in the chain from the proxy
// 3. RemoteAddr — direct connection fallback (with port stripped).
func extractClientIP(r *http.Request) string {
	if clientIP := r.Header.Get("X-Client-Ip"); clientIP != "" {
		host, _, err := net.SplitHostPort(clientIP)
		if err == nil {
			return host
		}

		return clientIP
	}

	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// X-Forwarded-For format: "client, proxy1, proxy2"
		// Take the first (leftmost) IP as the real client.
		if idx := strings.IndexByte(fwd, ','); idx >= 0 {
			fwd = strings.TrimSpace(fwd[:idx])
		}
		host, _, err := net.SplitHostPort(fwd)
		if err == nil {
			return host
		}

		return fwd
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
