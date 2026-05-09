package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	hsmv1 "github.com/ajamous/aether/services/hsm-broker/api/v1"
	"github.com/ajamous/aether/services/hsm-broker/internal/backend/memory"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	b := memory.New()
	s := New(b)
	return httptest.NewServer(s.Routes())
}

func postJSON(t *testing.T, url string, body any, dst any) *http.Response {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if dst != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return resp
}

func TestServer_Health(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var hr hsmv1.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&hr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !hr.Ready || hr.Backend != "memory" {
		t.Fatalf("unexpected health: %+v", hr)
	}
}

func TestServer_GenerateAndSign_RoundTrip(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	var gen hsmv1.GenerateKeyPairResponse
	resp := postJSON(t, srv.URL+"/v1/generate-key-pair", hsmv1.GenerateKeyPairRequest{
		Label: "DPpb",
		Kind:  hsmv1.KeyKindECDSA,
		Curve: hsmv1.CurveP256,
	}, &gen)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("generate status=%d", resp.StatusCode)
	}
	if gen.Handle.ID == "" {
		t.Fatal("expected non-empty key id")
	}

	digest := sha256.Sum256([]byte("hello"))
	var sig hsmv1.SignResponse
	resp = postJSON(t, srv.URL+"/v1/sign", hsmv1.SignRequest{
		KeyID:     gen.Handle.ID,
		Digest:    digest[:],
		DigestAlg: hsmv1.HashSHA256,
	}, &sig)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sign status=%d", resp.StatusCode)
	}
	if len(sig.SignatureDER) == 0 {
		t.Fatal("expected non-empty signature")
	}
}

func TestServer_Sign_KeyNotFound_404(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	digest := sha256.Sum256([]byte("hello"))
	resp := postJSON(t, srv.URL+"/v1/sign", hsmv1.SignRequest{
		KeyID:     "nonexistent",
		Digest:    digest[:],
		DigestAlg: hsmv1.HashSHA256,
	}, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/problem+json") {
		t.Fatalf("Content-Type = %q, want problem+json", resp.Header.Get("Content-Type"))
	}
}

func TestServer_BadJSON_400(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/sign", "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestServer_ShutdownOnContextCancel(t *testing.T) {
	b := memory.New()
	s := New(b)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.ListenAndServe(ctx, "127.0.0.1:0")
	}()
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("shutdown returned %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}
