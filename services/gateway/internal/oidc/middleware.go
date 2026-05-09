package oidc

import (
	"context"
	"net/http"
	"strings"
)

// AdminReporter is the callback invoked once per rejection on the
// /v1/* admin surface. The gateway uses it to advance a per-reason
// 401 counter — same shape as the ES2+ mTLS reporter.
type AdminReporter func(reason Reason)

// Middleware enforces OIDC on the admin /v1/* surface. When v is
// nil the middleware is a no-op pass-through, so callers who want
// OIDC disabled (lab default) do not pay the cost.
//
// /v1/health and /metrics bypass the gate unconditionally — they
// have to, so kube-probes and Prometheus can scrape unauthenticated.
// Anything outside /v1/* (notably /gsma/rsp2/*) bypasses too; that
// surface has its own auth (mTLS + rate-limit).
func Middleware(v *Verifier, reporter AdminReporter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if v == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !shouldEnforce(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			tok := bearerFromHeader(r.Header.Get("Authorization"))
			result, err := v.Verify(r.Context(), tok)
			if err != nil {
				reason := ReasonNoToken
				if ve, ok := err.(*VerifyError); ok {
					reason = ve.Reason
				}
				if reporter != nil {
					reporter(reason)
				}
				w.Header().Set("WWW-Authenticate", `Bearer realm="aether-admin"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(withResult(r.Context(), result)))
		})
	}
}

// shouldEnforce returns true for /v1/* paths that should be gated
// by OIDC. /v1/health, /v1/openapi.yaml, and /metrics are exempt;
// non-/v1 paths are also exempt (they have their own auth).
//
// /v1/openapi.yaml is intentionally public: operators and CLI
// tooling discover the API by fetching the spec, and the surface
// is documented on GitHub regardless. The API itself stays gated.
func shouldEnforce(path string) bool {
	if !strings.HasPrefix(path, "/v1/") {
		return false
	}
	if path == "/v1/health" || path == "/v1/openapi.yaml" {
		return false
	}
	return true
}

func bearerFromHeader(h string) string {
	const p = "Bearer "
	if !strings.HasPrefix(h, p) {
		return ""
	}
	return strings.TrimSpace(h[len(p):])
}

// resultKey is the context key for VerifyResult.
type resultKey struct{}

func withResult(ctx context.Context, r *VerifyResult) context.Context {
	return context.WithValue(ctx, resultKey{}, r)
}

// FromContext returns the verified token claims attached by
// Middleware, if any. Useful for downstream handlers that want to
// scope their reads by the authenticated subject.
func FromContext(ctx context.Context) (*VerifyResult, bool) {
	r, ok := ctx.Value(resultKey{}).(*VerifyResult)
	return r, ok
}
