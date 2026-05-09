package identity

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

	"github.com/ajamous/aether/pkg/certmgrclient"
)

func mintCert(t *testing.T) []byte {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "trust test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	return der
}

func TestFetchTrustMaterial_HappyPath(t *testing.T) {
	root := mintCert(t)
	intermediate := mintCert(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
		switch r.URL.Path {
		case "/v1/trust-store/pem":
			pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: root})
		case "/v1/intermediates/pem":
			pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: intermediate})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := certmgrclient.New(srv.URL)
	tm, err := FetchTrustMaterial(context.Background(), c)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if tm.RootCount() != 1 || tm.IntermediateCount() != 1 {
		t.Fatalf("unexpected counts: roots=%d intermediates=%d", tm.RootCount(), tm.IntermediateCount())
	}
	if tm.Roots == nil || tm.Intermediates == nil {
		t.Fatal("expected populated cert pools")
	}
}

func TestFetchTrustMaterial_RejectsEmptyTrustStore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
	}))
	defer srv.Close()

	c := certmgrclient.New(srv.URL)
	if _, err := FetchTrustMaterial(context.Background(), c); err == nil {
		t.Fatal("expected error on empty trust store")
	}
}
