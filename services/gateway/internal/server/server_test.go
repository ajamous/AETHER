package server

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ajamous/aether/services/gateway/internal/oidc"
)

func TestGateway_Health(t *testing.T) {
	s, _ := New(Config{ProfileBuilder: "http://pb", SMDPPlus: "http://smdp", CertMgr: "http://cm"})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if out["ready"] != true {
		t.Fatal("not ready")
	}
}

func TestGateway_DownloadOrder_HappyPath(t *testing.T) {
	s, _ := New(Config{})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()
	body, _ := json.Marshal(DownloadOrderRequest{ICCID: "8900000000000000001"})
	resp, err := http.Post(srv.URL+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out DownloadOrderResponse
	json.NewDecoder(resp.Body).Decode(&out)
	if out.ICCID != "8900000000000000001" {
		t.Fatalf("iccid not echoed: %q", out.ICCID)
	}
}

// TestGateway_DownloadOrder_PreparesProfile confirms an order that
// carries subscriber data is forwarded to smdp-plus's prepare
// endpoint with the right shape, and the returned ICCID flows back.
func TestGateway_DownloadOrder_PreparesProfile(t *testing.T) {
	var gotPath string
	var gotPrep smdpPrepareRequest
	smdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields() // mirror smdp-plus: catches shape drift
		if err := dec.Decode(&gotPrep); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(smdpPrepareResponse{
			ICCID:          gotPrep.Subscriber.ICCID,
			MatchingID:     "mid-abc123",
			ActivationCode: "LPA:1$smdp.test$mid-abc123",
		})
	}))
	defer smdp.Close()

	s, _ := New(Config{SMDPPlus: smdp.URL})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()

	body, _ := json.Marshal(DownloadOrderRequest{
		ICCID:       "8900000000000000007",
		ProfileType: "lab-mvno",
		Subscriber: &OrderSubscriber{
			IMSI: "001010000000007",
			Ki:   bytes.Repeat([]byte{0x11}, 16),
			OPc:  bytes.Repeat([]byte{0x22}, 16),
		},
	})
	resp, err := http.Post(srv.URL+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if gotPath != "/v1/profiles/prepare" {
		t.Errorf("forwarded path = %q", gotPath)
	}
	if gotPrep.Template != "lab-mvno" {
		t.Errorf("template = %q, want lab-mvno", gotPrep.Template)
	}
	if gotPrep.Subscriber.ICCID != "8900000000000000007" || gotPrep.Subscriber.IMSI != "001010000000007" {
		t.Errorf("subscriber not forwarded: %+v", gotPrep.Subscriber)
	}
	if !bytes.Equal(gotPrep.Subscriber.Ki, bytes.Repeat([]byte{0x11}, 16)) {
		t.Errorf("Ki not forwarded")
	}
	var out DownloadOrderResponse
	json.NewDecoder(resp.Body).Decode(&out)
	if out.ICCID != "8900000000000000007" {
		t.Errorf("response ICCID = %q", out.ICCID)
	}
	if out.MatchingID != "mid-abc123" {
		t.Errorf("matching_id not propagated: %q", out.MatchingID)
	}
	if out.ActivationCode != "LPA:1$smdp.test$mid-abc123" {
		t.Errorf("activation_code not propagated: %q", out.ActivationCode)
	}
}

// TestGateway_DownloadOrder_SubscriberWithoutSmdp confirms an order
// with profile data but no smdp-plus configured fails clearly rather
// than silently echoing.
func TestGateway_DownloadOrder_SubscriberWithoutSmdp(t *testing.T) {
	s, _ := New(Config{})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()
	body, _ := json.Marshal(DownloadOrderRequest{
		ICCID:      "8900000000000000007",
		Subscriber: &OrderSubscriber{IMSI: "001010000000007"},
	})
	resp, err := http.Post(srv.URL+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}

// TestGateway_DownloadOrder_PropagatesSmdpError confirms a non-2xx
// from smdp-plus prepare surfaces through the gateway.
func TestGateway_DownloadOrder_PropagatesSmdpError(t *testing.T) {
	smdp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"detail": "bad subscriber"})
	}))
	defer smdp.Close()
	s, _ := New(Config{SMDPPlus: smdp.URL})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()
	body, _ := json.Marshal(DownloadOrderRequest{
		ICCID:      "8900000000000000007",
		Subscriber: &OrderSubscriber{IMSI: "bad"},
	})
	resp, err := http.Post(srv.URL+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 propagated", resp.StatusCode)
	}
}

func TestGateway_DownloadOrder_RejectsEmpty(t *testing.T) {
	s, _ := New(Config{})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/gsma/rsp2/es2plus/downloadOrder", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGateway_ProxyToProfileBuilder(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/templates" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"templates":["lab-mvno"]}`))
	}))
	defer upstream.Close()

	s, _ := New(Config{ProfileBuilder: upstream.URL})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/templates")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string][]string
	json.NewDecoder(resp.Body).Decode(&out)
	if len(out["templates"]) != 1 || out["templates"][0] != "lab-mvno" {
		t.Fatalf("unexpected proxy result: %+v", out)
	}
}

// TestGateway_RateLimit_RejectsAfterBurst drives the wired
// middleware end to end: 3 requests with burst=2 → third gets
// 429, the rate-limit counter advances, admin paths are not
// rate-limited, and the counter is exposed on /metrics.
func TestGateway_RateLimit_RejectsAfterBurst(t *testing.T) {
	s, _ := New(Config{
		RateLimitRPS:   0.001, // effectively no refill within the test
		RateLimitBurst: 2,
	})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()

	// Burst: 2 successful, 3rd rejected with 429.
	for i := 0; i < 2; i++ {
		body, _ := json.Marshal(DownloadOrderRequest{ICCID: "8900000000000000001"})
		resp, err := http.Post(srv.URL+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
		status := resp.StatusCode
		resp.Body.Close()
		if status != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200", i+1, status)
		}
	}
	body, _ := json.Marshal(DownloadOrderRequest{ICCID: "8900000000000000001"})
	resp, err := http.Post(srv.URL+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("burst+1: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("burst+1 status = %d, want 429", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("Retry-After header missing on 429")
	}

	// Admin path bypasses: many requests stay 200 even after the
	// public surface is exhausted.
	for i := 0; i < 10; i++ {
		r, err := http.Get(srv.URL + "/v1/health")
		if err != nil {
			t.Fatalf("admin call %d: %v", i+1, err)
		}
		status := r.StatusCode
		r.Body.Close()
		if status != http.StatusOK {
			t.Fatalf("admin call %d: status = %d, want 200", i+1, status)
		}
	}

	// /metrics exposes the rejected counter.
	mr, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	defer mr.Body.Close()
	buf, _ := io.ReadAll(mr.Body)
	got := string(buf)
	if !strings.Contains(got, "aether_gateway_ratelimit_rejected_total") {
		t.Errorf("metrics output missing rate-limit counter:\n%s", got)
	}
	if !strings.Contains(got, `class="es2plus"} 1`) {
		t.Errorf("expected rate-limit counter for es2plus to be 1; got:\n%s", got)
	}
}

// TestGateway_OIDC_RejectsAdminWithoutBearer drives the wired OIDC
// middleware end to end: /v1/templates without a token returns 401,
// /v1/health bypasses, the per-reason counter advances, and the
// counter is exposed on /metrics.
func TestGateway_OIDC_RejectsAdminWithoutBearer(t *testing.T) {
	idp := newGatewayFakeIdP(t)
	v, err := oidc.Discover(context.Background(), oidc.Config{
		Issuer:   idp.URL,
		Audience: "aether-admin",
	})
	if err != nil {
		t.Fatalf("oidc discover: %v", err)
	}

	s, _ := New(Config{ProfileBuilder: "http://pb", OIDCVerifier: v})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()

	// /v1/health bypasses unauthenticated.
	resp, err := http.Get(srv.URL + "/v1/health")
	if err != nil {
		t.Fatalf("/v1/health: %v", err)
	}
	healthStatus := resp.StatusCode
	resp.Body.Close()
	if healthStatus != http.StatusOK {
		t.Errorf("/v1/health status = %d (must bypass OIDC)", healthStatus)
	}

	// /v1/templates without a Bearer is rejected.
	resp, err = http.Get(srv.URL + "/v1/templates")
	if err != nil {
		t.Fatalf("get /v1/templates: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/v1/templates status = %d, want 401", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("WWW-Authenticate"), "Bearer ") {
		t.Errorf("WWW-Authenticate = %q", resp.Header.Get("WWW-Authenticate"))
	}

	// Counter should reflect the no_token rejection.
	mr, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("/metrics: %v", err)
	}
	defer mr.Body.Close()
	buf, _ := io.ReadAll(mr.Body)
	got := string(buf)
	if !strings.Contains(got, "aether_gateway_admin_unauthorized_total") {
		t.Errorf("metrics output missing admin counter:\n%s", got)
	}
	if !strings.Contains(got, `reason="no_token"} 1`) {
		t.Errorf("expected no_token counter to be 1, got:\n%s", got)
	}
}

// TestGateway_OIDC_AcceptsValidBearer confirms a token signed by
// the IdP passes the gate and reaches the proxy logic.
func TestGateway_OIDC_AcceptsValidBearer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"templates":["lab-mvno"]}`))
	}))
	defer upstream.Close()

	idp := newGatewayFakeIdP(t)
	v, _ := oidc.Discover(context.Background(), oidc.Config{
		Issuer:   idp.URL,
		Audience: "aether-admin",
	})
	s, _ := New(Config{ProfileBuilder: upstream.URL, OIDCVerifier: v})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()

	tok := idp.Mint(t, map[string]any{
		"iss": idp.URL,
		"aud": "aether-admin",
		"sub": "operator-1",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(5 * time.Minute).Unix(),
	})
	req, _ := http.NewRequest("GET", srv.URL+"/v1/templates", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// gwFakeIdP is a minimal RS256 IdP for the server-level integration
// tests. We don't reuse oidc_test.go's fakeIdP because it lives in a
// different package.
type gwFakeIdP struct {
	URL    string
	priv   *rsa.PrivateKey
	kid    string
	server *httptest.Server
}

func newGatewayFakeIdP(t *testing.T) *gwFakeIdP {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	idp := &gwFakeIdP{priv: priv, kid: "rsa-1"}
	mux := http.NewServeMux()
	idp.server = httptest.NewServer(mux)
	idp.URL = idp.server.URL
	t.Cleanup(idp.server.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":   idp.URL,
			"jwks_uri": idp.URL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		nB := base64.RawURLEncoding.EncodeToString(priv.PublicKey.N.Bytes())
		var eBuf [4]byte
		binary.BigEndian.PutUint32(eBuf[:], uint32(priv.PublicKey.E))
		i := 0
		for i < 3 && eBuf[i] == 0 {
			i++
		}
		eB := base64.RawURLEncoding.EncodeToString(eBuf[i:])
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{"kty": "RSA", "kid": idp.kid, "use": "sig", "alg": "RS256", "n": nB, "e": eB},
			},
		})
	})
	return idp
}

// Mint signs a JWT with the IdP's RSA key.
func (i *gwFakeIdP) Mint(t *testing.T, payload map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "kid": i.kid, "typ": "JWT"}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(payload)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, i.priv, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("rsa sign: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// TestGateway_OpenAPISpec exposes the embedded OpenAPI 3.1 spec
// at /v1/openapi.yaml. The endpoint bypasses OIDC (operators
// discover the API before authenticating). Verifies content-type
// and structural shape (the spec must declare openapi 3.x and
// contain the gateway's title).
func TestGateway_OpenAPISpec(t *testing.T) {
	s, _ := New(Config{})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/openapi.yaml")
	if err != nil {
		t.Fatalf("get /v1/openapi.yaml: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/yaml") {
		t.Errorf("Content-Type = %q, want application/yaml...", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	got := string(body)
	for _, want := range []string{"openapi: 3.", "Aether Gateway API", "/gsma/rsp2/es2plus/downloadOrder", "/v1/templates"} {
		if !strings.Contains(got, want) {
			t.Errorf("spec missing %q", want)
		}
	}
}

// TestGateway_OpenAPI_BypassesOIDC confirms /v1/openapi.yaml is
// reachable without a Bearer when the OIDC gate is enabled —
// operators and CLI tooling need to discover the API before
// authenticating.
func TestGateway_OpenAPI_BypassesOIDC(t *testing.T) {
	idp := newGatewayFakeIdP(t)
	v, _ := oidc.Discover(context.Background(), oidc.Config{
		Issuer:   idp.URL,
		Audience: "aether-admin",
	})
	s, _ := New(Config{OIDCVerifier: v})
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/openapi.yaml")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d (must bypass OIDC); want 200", resp.StatusCode)
	}
}
