// Package ratelimit is a per-source-IP token-bucket rate limiter
// for the gateway's public-facing GSMA paths.
//
// Production posture: the limiter is meant as a defence-in-depth
// control behind an L7 LB that already enforces per-client quota.
// SAS-SM auditors look for *some* DoS-mitigation control on the
// public surface; this is that control.
//
// Key by RemoteAddr (the source as seen by the gateway). Behind
// an LB, that's the LB's IP — meaning the limit aggregates all
// inbound traffic. That is the safe default: trusting
// X-Forwarded-For without a trusted-proxy CIDR list is how
// rate-limit bypasses happen. Operators who want per-real-client
// limiting should configure the upstream LB.
package ratelimit

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// Reporter is the callback invoked once per rejection. The
// gateway uses it to advance a per-class 429 counter for the
// AetherGatewayRateLimited alert.
type Reporter func(class string)

// Limiter is a token-bucket per source IP.
type Limiter struct {
	rate  float64 // steady-state tokens/sec
	burst float64 // bucket capacity

	mu      sync.Mutex
	buckets map[string]*bucket

	now func() time.Time // injectable for tests
}

type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// New constructs a Limiter. rps must be > 0; burst must be >= 1.
func New(rps float64, burst int) *Limiter {
	if rps <= 0 || burst < 1 {
		return nil
	}
	return &Limiter{
		rate:    rps,
		burst:   float64(burst),
		buckets: make(map[string]*bucket, 64),
		now:     time.Now,
	}
}

// Allow returns true if a token is available for source. False
// means the caller should reject the request with 429.
func (l *Limiter) Allow(source string) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[source]
	if !ok {
		l.buckets[source] = &bucket{tokens: l.burst - 1, lastRefill: now}
		return true
	}

	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.lastRefill = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// SourceIP extracts a stable rate-limit key from the request.
// Strips the port from RemoteAddr. IPv6 forms like `[::1]:1234`
// are unwrapped to `::1`.
func SourceIP(r *http.Request) string {
	addr := r.RemoteAddr
	if addr == "" {
		return "unknown"
	}
	if addr[0] == '[' {
		if j := strings.Index(addr, "]"); j > 0 {
			return addr[1:j]
		}
	}
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}

// ClassifyPath returns the path class to rate-limit, or "" to
// let the request through unmetered. Today: only the public
// /gsma/rsp2/* surface is rate-limited; admin and metrics paths
// are exempt (same exclusion shape as the mTLS gate).
func ClassifyPath(r *http.Request) string {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/gsma/rsp2/es2plus/"):
		return "es2plus"
	case strings.HasPrefix(p, "/gsma/rsp2/es9plus/"):
		return "es9plus"
	}
	return ""
}

// Middleware wraps a handler with rate-limiting. When l is nil
// the middleware is a no-op pass-through, so callers who want
// rate-limiting disabled (lab default) do not pay the cost.
func Middleware(l *Limiter, reporter Reporter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if l == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			class := ClassifyPath(r)
			if class == "" {
				next.ServeHTTP(w, r)
				return
			}
			if !l.Allow(SourceIP(r)) {
				if reporter != nil {
					reporter(class)
				}
				w.Header().Set("Retry-After", "1")
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
