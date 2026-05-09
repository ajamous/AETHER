package certmgrclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mintCert(t *testing.T) []byte {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageCertSign,
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return der
}

func TestTrustStore_FetchesAndParses(t *testing.T) {
	cert1 := mintCert(t)
	cert2 := mintCert(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/trust-store/pem" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		_ = pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: cert1})
		_ = pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: cert2})
	}))
	defer srv.Close()

	c := New(srv.URL)
	got, err := c.TrustStore(context.Background())
	if err != nil {
		t.Fatalf("trust store: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 certs, got %d", len(got))
	}
}

func TestIntermediates_EmptyOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
	}))
	defer srv.Close()

	c := New(srv.URL)
	got, err := c.Intermediates(context.Background())
	if err != nil {
		t.Fatalf("intermediates: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestNon2xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(srv.URL)
	if _, err := c.TrustStore(context.Background()); err == nil {
		t.Fatal("expected error on 500")
	}
}
