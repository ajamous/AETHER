package hsmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth_RoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(HealthResponse{Ready: true, Backend: "memory"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	got, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if !got.Ready || got.Backend != "memory" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestGenerateKeyPair_PostsAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/generate-key-pair" || r.Method != http.MethodPost {
			http.Error(w, "wrong route", http.StatusNotFound)
			return
		}
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["label"] != "DPauth" || req["kind"] != "ECDSA" || req["curve"] != "P256" {
			http.Error(w, "wrong payload", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GenerateKeyPairResponse{
			Handle:    KeyHandle{ID: "abcd", Label: "DPauth", Kind: KeyKindECDSA, Curve: CurveP256},
			PublicKey: []byte{0x04, 0x01, 0x02},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	got, err := c.GenerateKeyPair(context.Background(), "DPauth", KeyKindECDSA, CurveP256)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if got.Handle.ID != "abcd" || string(got.PublicKey) != "\x04\x01\x02" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestSign_PostsDigest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sign" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			KeyID     string `json:"key_id"`
			Digest    []byte `json:"digest"`
			DigestAlg string `json:"digest_alg"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.KeyID != "k1" || req.DigestAlg != "SHA256" || len(req.Digest) != 32 {
			http.Error(w, "wrong payload", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SignResponse{SignatureDER: []byte{0x30, 0x06}})
	}))
	defer srv.Close()

	c := New(srv.URL)
	digest := make([]byte, 32)
	got, err := c.Sign(context.Background(), "k1", digest)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if len(got.SignatureDER) != 2 {
		t.Fatalf("unexpected sig: %x", got.SignatureDER)
	}
}

func TestNon2xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"detail": "key not found"})
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Sign(context.Background(), "missing", make([]byte, 32))
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Fatalf("expected IsNotFound, got %v", err)
	}
}
