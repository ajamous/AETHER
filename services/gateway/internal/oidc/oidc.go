// Package oidc verifies OpenID Connect ID tokens on the gateway's
// admin (/v1/*) surface. Two surfaces are protected differently:
//
//   - /gsma/rsp2/* uses path-scoped mTLS (services/gateway/internal/tlsconf).
//     The BSS authenticates with a client certificate.
//   - /v1/* uses Bearer-token OIDC (this package). The operator UI's
//     server-side fetches forward the user's ID token; CLI clients
//     use Authorization: Bearer <jwt>.
//
// /v1/health and /metrics are intentionally exempt — kube-probes and
// Prometheus scrape unauthenticated, by design.
//
// Production posture: the verifier checks the signature against the
// IdP's JWKS, requires iss + aud to match the configured values, and
// rejects expired or not-yet-valid tokens. RS256 and ES256 are
// supported; other algorithms (HS*, RS384, RS512, ES384, ES512, EdDSA)
// are out of scope for the SAS-SM admin path. Symmetric algorithms
// in particular are deliberately rejected — admin tokens must be
// asymmetrically signed by the IdP.
//
// The package is stdlib-only on purpose: a fourth-party JWT library
// would be a non-trivial supply-chain surface for the
// SAS-SM-relevant admin auth gate.
package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Verifier validates OIDC ID tokens against a configured issuer +
// audience.
type Verifier struct {
	issuer   string
	audience string

	httpClient *http.Client

	// jwks cache. Refreshed on cache miss (unknown kid) or every
	// jwksTTL, whichever comes first.
	jwksURL string
	jwksTTL time.Duration
	mu      sync.RWMutex
	jwksAt  time.Time
	keys    map[string]any // kid → *rsa.PublicKey or *ecdsa.PublicKey

	// now is the clock used for exp/nbf checks. Injectable for tests.
	now func() time.Time
}

// Config configures a Verifier.
type Config struct {
	// Issuer is the OIDC issuer URL. Must match the `iss` claim in
	// every accepted token. Required.
	Issuer string

	// Audience is the audience that admin tokens must include.
	// Typically the operator UI's client ID. Required.
	Audience string

	// JWKSTTL is how long to cache fetched JWKS. Zero defaults to
	// 5 minutes. Misses on a kid trigger an immediate refresh
	// regardless of the TTL.
	JWKSTTL time.Duration

	// HTTPClient is used for discovery and JWKS fetches. Zero
	// defaults to http.DefaultClient.
	HTTPClient *http.Client
}

// Discover fetches the issuer's /.well-known/openid-configuration
// document, extracts jwks_uri, and returns a Verifier with that URL
// preloaded. The JWKS itself is not fetched until first use.
func Discover(ctx context.Context, cfg Config) (*Verifier, error) {
	if cfg.Issuer == "" {
		return nil, errors.New("oidc: Issuer required")
	}
	if cfg.Audience == "" {
		return nil, errors.New("oidc: Audience required")
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	url := strings.TrimRight(cfg.Issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("oidc: build discovery request: %w", err)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc: discovery fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc: discovery returned %d", resp.StatusCode)
	}
	var doc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("oidc: decode discovery: %w", err)
	}
	if doc.Issuer != cfg.Issuer {
		return nil, fmt.Errorf("oidc: discovery issuer %q does not match configured %q", doc.Issuer, cfg.Issuer)
	}
	if doc.JWKSURI == "" {
		return nil, errors.New("oidc: discovery missing jwks_uri")
	}
	ttl := cfg.JWKSTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Verifier{
		issuer:     cfg.Issuer,
		audience:   cfg.Audience,
		httpClient: hc,
		jwksURL:    doc.JWKSURI,
		jwksTTL:    ttl,
		keys:       map[string]any{},
		now:        time.Now,
	}, nil
}

// NewWithJWKS builds a Verifier directly from a JWKS URL, skipping
// discovery. Useful when the issuer URL and JWKS URL diverge (some
// IdP deployments split them).
func NewWithJWKS(jwksURL string, cfg Config) (*Verifier, error) {
	if cfg.Issuer == "" || cfg.Audience == "" || jwksURL == "" {
		return nil, errors.New("oidc: Issuer, Audience, and jwksURL required")
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	ttl := cfg.JWKSTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Verifier{
		issuer:     cfg.Issuer,
		audience:   cfg.Audience,
		httpClient: hc,
		jwksURL:    jwksURL,
		jwksTTL:    ttl,
		keys:       map[string]any{},
		now:        time.Now,
	}, nil
}

// VerifyResult is the parsed token's claims after a successful Verify.
type VerifyResult struct {
	Subject  string
	Issuer   string
	Audience []string
	Expiry   time.Time
	IssuedAt time.Time
	Claims   map[string]any
}

// Reason classifies a verification failure for the 401 counter.
type Reason string

const (
	ReasonNoToken         Reason = "no_token"
	ReasonMalformed       Reason = "malformed"
	ReasonUnsupportedAlg  Reason = "unsupported_alg"
	ReasonUnknownKID      Reason = "unknown_kid"
	ReasonBadSignature    Reason = "bad_signature"
	ReasonWrongIssuer     Reason = "wrong_issuer"
	ReasonWrongAudience   Reason = "wrong_audience"
	ReasonExpired         Reason = "expired"
	ReasonNotYetValid     Reason = "not_yet_valid"
	ReasonJWKSFetchFailed Reason = "jwks_fetch_failed"
)

// VerifyError carries a Reason so the middleware can label its
// counter without parsing strings.
type VerifyError struct {
	Reason Reason
	Err    error
}

func (e *VerifyError) Error() string {
	if e.Err == nil {
		return string(e.Reason)
	}
	return string(e.Reason) + ": " + e.Err.Error()
}

func wrap(reason Reason, err error) *VerifyError { return &VerifyError{Reason: reason, Err: err} }

// Verify parses and validates a compact-serialised JWT. On success
// returns the parsed claims; on failure returns a *VerifyError with
// a Reason suitable for the 401 counter.
//
// Validation steps:
//  1. Split into three base64url segments
//  2. Header alg ∈ {RS256, ES256} and kid present
//  3. JWKS lookup for kid (with refresh-on-miss)
//  4. Signature verifies against the matching public key
//  5. iss == configured issuer
//  6. aud contains configured audience
//  7. exp > now (with no clock skew tolerance — IdP clocks should
//     be NTP-aligned in production); nbf <= now if present
func (v *Verifier) Verify(ctx context.Context, token string) (*VerifyResult, error) {
	if token == "" {
		return nil, wrap(ReasonNoToken, nil)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, wrap(ReasonMalformed, errors.New("expected 3 dot-separated segments"))
	}
	headerB, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, wrap(ReasonMalformed, fmt.Errorf("header b64: %w", err))
	}
	payloadB, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, wrap(ReasonMalformed, fmt.Errorf("payload b64: %w", err))
	}
	sigB, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, wrap(ReasonMalformed, fmt.Errorf("sig b64: %w", err))
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerB, &header); err != nil {
		return nil, wrap(ReasonMalformed, fmt.Errorf("header json: %w", err))
	}
	if header.Alg != "RS256" && header.Alg != "ES256" {
		return nil, wrap(ReasonUnsupportedAlg, fmt.Errorf("alg=%q not in {RS256, ES256}", header.Alg))
	}
	if header.Kid == "" {
		return nil, wrap(ReasonMalformed, errors.New("header missing kid"))
	}

	pub, err := v.lookupKey(ctx, header.Kid)
	if err != nil {
		var ve *VerifyError
		if errors.As(err, &ve) {
			return nil, ve
		}
		return nil, wrap(ReasonJWKSFetchFailed, err)
	}

	signingInput := []byte(parts[0] + "." + parts[1])
	digest := sha256.Sum256(signingInput)

	switch header.Alg {
	case "RS256":
		rsaKey, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, wrap(ReasonBadSignature, fmt.Errorf("kid %q is not an RSA key", header.Kid))
		}
		if err := rsa.VerifyPKCS1v15(rsaKey, crypto.SHA256, digest[:], sigB); err != nil {
			return nil, wrap(ReasonBadSignature, err)
		}
	case "ES256":
		ecKey, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return nil, wrap(ReasonBadSignature, fmt.Errorf("kid %q is not an ECDSA key", header.Kid))
		}
		if ecKey.Curve != elliptic.P256() {
			return nil, wrap(ReasonBadSignature, fmt.Errorf("ES256 requires P-256, got %s", ecKey.Curve.Params().Name))
		}
		if len(sigB) != 64 {
			return nil, wrap(ReasonBadSignature, fmt.Errorf("ES256 sig must be 64 raw bytes (R||S), got %d", len(sigB)))
		}
		r := new(big.Int).SetBytes(sigB[:32])
		s := new(big.Int).SetBytes(sigB[32:])
		if !ecdsa.Verify(ecKey, digest[:], r, s) {
			return nil, wrap(ReasonBadSignature, errors.New("ECDSA verify failed"))
		}
	}

	var claims map[string]any
	if err := json.Unmarshal(payloadB, &claims); err != nil {
		return nil, wrap(ReasonMalformed, fmt.Errorf("payload json: %w", err))
	}

	if iss, _ := claims["iss"].(string); iss != v.issuer {
		return nil, wrap(ReasonWrongIssuer, fmt.Errorf("iss=%q want %q", iss, v.issuer))
	}
	if !audienceMatches(claims["aud"], v.audience) {
		return nil, wrap(ReasonWrongAudience, fmt.Errorf("aud does not include %q", v.audience))
	}
	exp, ok := claimAsTime(claims["exp"])
	if !ok {
		return nil, wrap(ReasonMalformed, errors.New("missing or invalid exp"))
	}
	now := v.now()
	if !exp.After(now) {
		return nil, wrap(ReasonExpired, fmt.Errorf("exp=%s now=%s", exp.Format(time.RFC3339), now.Format(time.RFC3339)))
	}
	if nbf, ok := claimAsTime(claims["nbf"]); ok && nbf.After(now) {
		return nil, wrap(ReasonNotYetValid, fmt.Errorf("nbf=%s now=%s", nbf.Format(time.RFC3339), now.Format(time.RFC3339)))
	}
	iat, _ := claimAsTime(claims["iat"])

	auds := audienceSlice(claims["aud"])
	sub, _ := claims["sub"].(string)
	return &VerifyResult{
		Subject:  sub,
		Issuer:   v.issuer,
		Audience: auds,
		Expiry:   exp,
		IssuedAt: iat,
		Claims:   claims,
	}, nil
}

// lookupKey returns the public key for kid, refreshing the JWKS if
// the kid is unknown or the cache is stale.
func (v *Verifier) lookupKey(ctx context.Context, kid string) (any, error) {
	v.mu.RLock()
	pub, ok := v.keys[kid]
	stale := time.Since(v.jwksAt) > v.jwksTTL
	v.mu.RUnlock()
	if ok && !stale {
		return pub, nil
	}
	if err := v.refreshJWKS(ctx); err != nil {
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	pub, ok = v.keys[kid]
	if !ok {
		return nil, wrap(ReasonUnknownKID, fmt.Errorf("kid %q not in JWKS", kid))
	}
	return pub, nil
}

func (v *Verifier) refreshJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("oidc: build jwks request: %w", err)
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("oidc: jwks fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("oidc: jwks returned %d: %s", resp.StatusCode, string(body))
	}
	var doc struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("oidc: decode jwks: %w", err)
	}

	keys := make(map[string]any, len(doc.Keys))
	for _, raw := range doc.Keys {
		var jwk struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			// RSA
			N string `json:"n"`
			E string `json:"e"`
			// EC
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		}
		if err := json.Unmarshal(raw, &jwk); err != nil {
			continue
		}
		if jwk.Kid == "" {
			continue
		}
		// Skip keys not usable for signing.
		if jwk.Use != "" && jwk.Use != "sig" {
			continue
		}
		switch jwk.Kty {
		case "RSA":
			pub, err := jwkRSA(jwk.N, jwk.E)
			if err == nil {
				keys[jwk.Kid] = pub
			}
		case "EC":
			pub, err := jwkEC(jwk.Crv, jwk.X, jwk.Y)
			if err == nil {
				keys[jwk.Kid] = pub
			}
		}
	}

	v.mu.Lock()
	v.keys = keys
	v.jwksAt = time.Now()
	v.mu.Unlock()
	return nil
}

func jwkRSA(nStr, eStr string) (*rsa.PublicKey, error) {
	nB, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, err
	}
	eB, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, err
	}
	// Right-pad e to 4 bytes for binary.BigEndian.Uint32.
	var ePadded [4]byte
	copy(ePadded[4-len(eB):], eB)
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nB),
		E: int(binary.BigEndian.Uint32(ePadded[:])),
	}, nil
}

func jwkEC(crv, xStr, yStr string) (*ecdsa.PublicKey, error) {
	if crv != "P-256" {
		return nil, fmt.Errorf("oidc: only EC P-256 supported, got %q", crv)
	}
	xB, err := base64.RawURLEncoding.DecodeString(xStr)
	if err != nil {
		return nil, err
	}
	yB, err := base64.RawURLEncoding.DecodeString(yStr)
	if err != nil {
		return nil, err
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xB),
		Y:     new(big.Int).SetBytes(yB),
	}, nil
}
