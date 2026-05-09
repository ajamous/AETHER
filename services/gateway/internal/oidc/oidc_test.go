package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeIdP stands up a minimal OIDC discovery + JWKS surface for
// tests. Keys are generated per-IdP; tokens can be signed using
// either rsa or ecdsa.
type fakeIdP struct {
	srv       *httptest.Server
	rsaPriv   *rsa.PrivateKey
	rsaKID    string
	ecdsaPriv *ecdsa.PrivateKey
	ecdsaKID  string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	ecdsaPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa keygen: %v", err)
	}
	idp := &fakeIdP{
		rsaPriv:   rsaPriv,
		rsaKID:    "rsa-1",
		ecdsaPriv: ecdsaPriv,
		ecdsaKID:  "ec-1",
	}

	mux := http.NewServeMux()
	idp.srv = httptest.NewServer(mux)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":   idp.srv.URL,
			"jwks_uri": idp.srv.URL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		nB := base64.RawURLEncoding.EncodeToString(rsaPriv.PublicKey.N.Bytes())
		var eBuf [4]byte
		binary.BigEndian.PutUint32(eBuf[:], uint32(rsaPriv.PublicKey.E))
		// Trim leading zero bytes for RFC 7518 minimal encoding.
		i := 0
		for i < 3 && eBuf[i] == 0 {
			i++
		}
		eB := base64.RawURLEncoding.EncodeToString(eBuf[i:])
		xB := base64.RawURLEncoding.EncodeToString(ecdsaPriv.PublicKey.X.Bytes())
		yB := base64.RawURLEncoding.EncodeToString(ecdsaPriv.PublicKey.Y.Bytes())
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{"kty": "RSA", "kid": idp.rsaKID, "use": "sig", "alg": "RS256", "n": nB, "e": eB},
				{"kty": "EC", "kid": idp.ecdsaKID, "use": "sig", "alg": "ES256", "crv": "P-256", "x": xB, "y": yB},
			},
		})
	})

	t.Cleanup(idp.srv.Close)
	return idp
}

// signRS256 produces a JWT signed with the IdP's RSA key.
func (i *fakeIdP) signRS256(t *testing.T, payload map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "kid": i.rsaKID, "typ": "JWT"}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(payload)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, i.rsaPriv, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("rsa sign: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// signES256 produces a JWT signed with the IdP's ECDSA key.
func (i *fakeIdP) signES256(t *testing.T, payload map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "ES256", "kid": i.ecdsaKID, "typ": "JWT"}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(payload)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, i.ecdsaPriv, digest[:])
	if err != nil {
		t.Fatalf("ecdsa sign: %v", err)
	}
	// JWS ES256 signature is raw R||S, fixed 64 bytes.
	rBytes, sBytes := r.Bytes(), s.Bytes()
	sig := make([]byte, 64)
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):], sBytes)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// makePayload returns a baseline well-formed claim set the caller
// can mutate for negative-path tests.
func makePayload(idp *fakeIdP, audience string) map[string]any {
	now := time.Now().Unix()
	return map[string]any{
		"iss": idp.srv.URL,
		"aud": audience,
		"sub": "operator-1",
		"iat": now,
		"nbf": now - 5,
		"exp": now + 600,
	}
}

func newVerifier(t *testing.T, idp *fakeIdP, audience string) *Verifier {
	t.Helper()
	v, err := Discover(context.Background(), Config{
		Issuer:   idp.srv.URL,
		Audience: audience,
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	return v
}

func TestVerify_RS256_HappyPath(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")
	tok := idp.signRS256(t, makePayload(idp, "aether-admin"))

	res, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if res.Subject != "operator-1" {
		t.Errorf("subject = %q", res.Subject)
	}
	if res.Issuer != idp.srv.URL {
		t.Errorf("issuer = %q", res.Issuer)
	}
}

func TestVerify_ES256_HappyPath(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")
	tok := idp.signES256(t, makePayload(idp, "aether-admin"))

	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerify_RejectsHS256(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")

	header := map[string]any{"alg": "HS256", "kid": idp.rsaKID, "typ": "JWT"}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(makePayload(idp, "aether-admin"))
	tok := base64.RawURLEncoding.EncodeToString(hb) + "." +
		base64.RawURLEncoding.EncodeToString(pb) + ".AAAA"

	_, err := v.Verify(context.Background(), tok)
	assertReason(t, err, ReasonUnsupportedAlg)
}

func TestVerify_RejectsExpired(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")
	payload := makePayload(idp, "aether-admin")
	payload["exp"] = time.Now().Add(-1 * time.Minute).Unix()
	tok := idp.signRS256(t, payload)

	_, err := v.Verify(context.Background(), tok)
	assertReason(t, err, ReasonExpired)
}

func TestVerify_RejectsNotYetValid(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")
	payload := makePayload(idp, "aether-admin")
	payload["nbf"] = time.Now().Add(10 * time.Minute).Unix()
	tok := idp.signRS256(t, payload)

	_, err := v.Verify(context.Background(), tok)
	assertReason(t, err, ReasonNotYetValid)
}

func TestVerify_RejectsWrongIssuer(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")
	payload := makePayload(idp, "aether-admin")
	payload["iss"] = "https://evil.example"
	tok := idp.signRS256(t, payload)

	_, err := v.Verify(context.Background(), tok)
	assertReason(t, err, ReasonWrongIssuer)
}

func TestVerify_RejectsWrongAudience(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")
	payload := makePayload(idp, "wrong-aud")
	tok := idp.signRS256(t, payload)

	_, err := v.Verify(context.Background(), tok)
	assertReason(t, err, ReasonWrongAudience)
}

func TestVerify_AcceptsAudienceArrayContainingTarget(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")
	payload := makePayload(idp, "")
	payload["aud"] = []any{"some-other", "aether-admin", "yet-another"}
	tok := idp.signRS256(t, payload)

	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("verify (aud array containing target): %v", err)
	}
}

func TestVerify_RejectsTamperedSignature(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")
	tok := idp.signRS256(t, makePayload(idp, "aether-admin"))
	// flip a byte in the signature
	parts := strings.Split(tok, ".")
	sigB, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sigB[0] ^= 0x55
	tampered := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sigB)

	_, err := v.Verify(context.Background(), tampered)
	assertReason(t, err, ReasonBadSignature)
}

func TestVerify_RejectsUnknownKID(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")

	header := map[string]any{"alg": "RS256", "kid": "does-not-exist", "typ": "JWT"}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(makePayload(idp, "aether-admin"))
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)
	digest := sha256.Sum256([]byte(signingInput))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, idp.rsaPriv, crypto.SHA256, digest[:])
	tok := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)

	_, err := v.Verify(context.Background(), tok)
	assertReason(t, err, ReasonUnknownKID)
}

func TestVerify_RejectsMalformed(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")

	for _, bad := range []string{
		"",
		"not-a-jwt",
		"only.two",
		"bad-b64.eyJ9.AAAA",
		"AAAA.bad-b64.AAAA",
	} {
		_, err := v.Verify(context.Background(), bad)
		if err == nil {
			t.Errorf("input %q: expected error", bad)
			continue
		}
		ve, _ := err.(*VerifyError)
		if ve == nil || (ve.Reason != ReasonNoToken && ve.Reason != ReasonMalformed) {
			t.Errorf("input %q: reason = %v want NoToken or Malformed", bad, ve)
		}
	}
}

func TestVerify_DiscoveryRejectsMismatchedIssuer(t *testing.T) {
	idp := newFakeIdP(t)
	_, err := Discover(context.Background(), Config{
		Issuer:   "http://wrong.example",
		Audience: "x",
	})
	if err == nil {
		t.Fatal("expected discovery to fail against the wrong issuer URL")
	}
	// Sanity: discovery against the right URL succeeds.
	if _, err := Discover(context.Background(), Config{
		Issuer:   idp.srv.URL,
		Audience: "x",
	}); err != nil {
		t.Fatalf("right-issuer discover: %v", err)
	}
}

func assertReason(t *testing.T, err error, want Reason) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with reason %s, got nil", want)
	}
	ve, ok := err.(*VerifyError)
	if !ok {
		t.Fatalf("expected *VerifyError, got %T: %v", err, err)
	}
	if ve.Reason != want {
		t.Errorf("reason = %s, want %s (err: %v)", ve.Reason, want, err)
	}
}

// --- middleware -----------------------------------------------------------

func TestMiddleware_HappyPath(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")

	mw := Middleware(v, nil)
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		// Subject should be available via context.
		res, ok := FromContext(r.Context())
		if !ok || res.Subject != "operator-1" {
			t.Errorf("FromContext = %+v ok=%v", res, ok)
		}
		w.WriteHeader(http.StatusOK)
	}))

	tok := idp.signRS256(t, makePayload(idp, "aether-admin"))
	r := httptest.NewRequest("GET", "/v1/templates", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !called {
		t.Fatal("downstream handler not invoked")
	}
}

func TestMiddleware_HealthBypasses(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")
	mw := Middleware(v, nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/v1/health", "/metrics", "/gsma/rsp2/es2plus/downloadOrder"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status %d, want 200 (no token but bypass expected)", path, w.Code)
		}
	}
}

func TestMiddleware_RejectsMissingToken(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")
	var seen Reason
	mw := Middleware(v, func(reason Reason) { seen = reason })
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream must not be invoked when token is missing")
	}))

	r := httptest.NewRequest("GET", "/v1/templates", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if seen != ReasonNoToken {
		t.Errorf("reporter saw %s, want %s", seen, ReasonNoToken)
	}
	if !strings.HasPrefix(w.Header().Get("WWW-Authenticate"), "Bearer ") {
		t.Errorf("WWW-Authenticate = %q", w.Header().Get("WWW-Authenticate"))
	}
}

func TestMiddleware_NilVerifierPassesThrough(t *testing.T) {
	mw := Middleware(nil, nil)
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest("GET", "/v1/templates", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !called || w.Code != http.StatusOK {
		t.Fatalf("nil verifier must pass through (called=%v code=%d)", called, w.Code)
	}
}

// Sanity check: verify() resolves an unknown kid by re-fetching JWKS.
// Mostly a sanity that our cache plumbing doesn't get stuck.
func TestVerify_RefreshesJWKSOnUnknownKID(t *testing.T) {
	idp := newFakeIdP(t)
	v := newVerifier(t, idp, "aether-admin")

	// Force a stale cache entry that doesn't include the kid yet.
	v.mu.Lock()
	v.keys = map[string]any{}
	v.jwksAt = time.Now().Add(-1 * time.Hour) // already-stale
	v.mu.Unlock()

	tok := idp.signRS256(t, makePayload(idp, "aether-admin"))
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("verify after forced cache flush: %v", err)
	}
}

// TestNewWithJWKS is a smoke test for the alternate constructor.
func TestNewWithJWKS(t *testing.T) {
	idp := newFakeIdP(t)
	v, err := NewWithJWKS(idp.srv.URL+"/jwks", Config{
		Issuer:   idp.srv.URL,
		Audience: "aether-admin",
	})
	if err != nil {
		t.Fatalf("NewWithJWKS: %v", err)
	}
	tok := idp.signRS256(t, makePayload(idp, "aether-admin"))
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("verify via NewWithJWKS: %v", err)
	}
}

// --- helpers --------------------------------------------------------------

// padBigEndian ensures a byte slice is exactly n bytes (left-padded
// with zeros). Used in the test if we ever round-trip a big.Int.
func padBigEndian(b []byte, n int) []byte {
	if len(b) >= n {
		return b
	}
	out := make([]byte, n)
	copy(out[n-len(b):], b)
	return out
}

// sanityRoundTripBigInt is a guard against accidentally regressing
// big-endian handling in claims.go.
func sanityRoundTripBigInt(t *testing.T) {
	t.Helper()
	x := big.NewInt(0x12345678)
	if got := fmt.Sprintf("%x", padBigEndian(x.Bytes(), 4)); got != "12345678" {
		t.Fatalf("padBigEndian round-trip broken: %s", got)
	}
}
