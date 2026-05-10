package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ajamous/aether/services/certmgr/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	chain, err := store.GenerateLabChain()
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	dir := t.TempDir()
	if err := chain.WriteFiles(dir, os.WriteFile); err != nil {
		t.Fatalf("write: %v", err)
	}
	st, err := store.New(store.Config{
		Mode:              store.ModeLab,
		TrustStorePath:    filepath.Join(dir, "ci-roots.pem"),
		IntermediatesPath: filepath.Join(dir, "eum.pem"),
		IdentityPaths: map[store.Identity]string{
			store.IdentityDPTLS:  filepath.Join(dir, "DPtls.pem"),
			store.IdentityDPAuth: filepath.Join(dir, "DPauth.pem"),
			store.IdentityDPpb:   filepath.Join(dir, "DPpb.pem"),
		},
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	return st
}

func TestServer_Health(t *testing.T) {
	srv := httptest.NewServer(New(newTestStore(t)).Routes())
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
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["mode"] != "lab" {
		t.Fatalf("mode = %v", out["mode"])
	}
	if out["ready"] != true {
		t.Fatalf("ready = %v", out["ready"])
	}
}

func TestServer_ListCerts(t *testing.T) {
	srv := httptest.NewServer(New(newTestStore(t)).Routes())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/certs")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 identity certs, got %d", len(list))
	}
}

func TestServer_GetCert_PEM(t *testing.T) {
	srv := httptest.NewServer(New(newTestStore(t)).Routes())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/certs/DPtls")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(body), "-----BEGIN CERTIFICATE-----") {
		t.Fatalf("expected PEM body, got %q", string(body[:60]))
	}
}

func TestServer_TrustStorePEM_AndIntermediatesPEM(t *testing.T) {
	srv := httptest.NewServer(New(newTestStore(t)).Routes())
	defer srv.Close()

	for _, path := range []string{"/v1/trust-store/pem", "/v1/intermediates/pem"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			t.Fatalf("status %s = %d", path, resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.HasPrefix(string(body), "-----BEGIN CERTIFICATE-----") {
			t.Fatalf("%s: expected PEM, got %q", path, string(body[:60]))
		}
	}
}

func TestServer_GetCert_NotFound(t *testing.T) {
	srv := httptest.NewServer(New(newTestStore(t)).Routes())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/certs/Nonsense")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestServer_Metrics_Shape(t *testing.T) {
	srv := httptest.NewServer(New(newTestStore(t)).Routes())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{
		"aether_certmgr_mode",
		"aether_cert_expiry_days",
		"aether_cert_loaded",
		"aether_trust_store_size",
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("metrics missing %q\nbody:\n%s", want, body)
		}
	}
}
