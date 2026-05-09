package server

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajamous/aether/services/gateway/internal/tlsconf"
)

// labPKI mints a small PKI in process: a server cert (for the
// gateway listener) and two client certs (one trusted, one signed
// by an unrelated CA). A test that hits a TLS gateway with the
// "untrusted" client cert should be rejected on ES2+.
type labPKI struct {
	serverRoot   *x509.Certificate
	serverRootK  *ecdsa.PrivateKey
	serverCert   *x509.Certificate
	serverCertK  *ecdsa.PrivateKey
	clientRoot   *x509.Certificate
	clientRootK  *ecdsa.PrivateKey
	clientCert   *x509.Certificate
	clientCertK  *ecdsa.PrivateKey
	otherRoot    *x509.Certificate
	otherRootK   *ecdsa.PrivateKey
	otherClient  *x509.Certificate
	otherClientK *ecdsa.PrivateKey
}

func mkCA(t *testing.T, cn string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	signer := tpl
	signerK := key
	if parent != nil {
		signer, signerK = parent, parentKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, signer, &key.PublicKey, signerK)
	if err != nil {
		t.Fatalf("create CA %q: %v", cn, err)
	}
	c, _ := x509.ParseCertificate(der)
	return c, key
}

func mkLeaf(t *testing.T, cn string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey, isClient bool) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	if isClient {
		tpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else {
		tpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		tpl.DNSNames = []string{"localhost"}
		tpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, parent, &key.PublicKey, parentKey)
	if err != nil {
		t.Fatalf("create leaf %q: %v", cn, err)
	}
	c, _ := x509.ParseCertificate(der)
	return c, key
}

func newLabPKI(t *testing.T) *labPKI {
	t.Helper()
	srvRoot, srvRootK := mkCA(t, "Test Server CA", nil, nil)
	srvCert, srvCertK := mkLeaf(t, "127.0.0.1", srvRoot, srvRootK, false)

	cliRoot, cliRootK := mkCA(t, "Test Client CA", nil, nil)
	cliCert, cliCertK := mkLeaf(t, "Test BSS Client", cliRoot, cliRootK, true)

	otherRoot, otherRootK := mkCA(t, "Unrelated CA", nil, nil)
	otherCert, otherCertK := mkLeaf(t, "Rogue BSS Client", otherRoot, otherRootK, true)

	return &labPKI{
		serverRoot: srvRoot, serverRootK: srvRootK,
		serverCert: srvCert, serverCertK: srvCertK,
		clientRoot: cliRoot, clientRootK: cliRootK,
		clientCert: cliCert, clientCertK: cliCertK,
		otherRoot: otherRoot, otherRootK: otherRootK,
		otherClient: otherCert, otherClientK: otherCertK,
	}
}

func writePEM(t *testing.T, dir, name string, der []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("encode %s: %v", name, err)
	}
	return p
}

func writeKeyPEM(t *testing.T, dir, name string, key *ecdsa.PrivateKey) string {
	t.Helper()
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	defer f.Close()
	der, _ := x509.MarshalECPrivateKey(key)
	if err := pem.Encode(f, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}); err != nil {
		t.Fatalf("encode %s: %v", name, err)
	}
	return p
}

// startTLSServer brings up a *Server with the supplied PKI's server
// cert and the client root in its ES2+ trust store. Returns the
// listener URL and a TLS-aware http.Client for "trusted" client.
func startTLSServer(t *testing.T, pki *labPKI, withMTLS bool) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	certPath := writePEM(t, dir, "server.crt", pki.serverCert.Raw)
	keyPath := writeKeyPEM(t, dir, "server.key", pki.serverCertK)
	caPath := ""
	if withMTLS {
		caPath = writePEM(t, dir, "client-ca.crt", pki.clientRoot.Raw)
	}

	srv, err := New(Config{
		TLS: tlsconf.Config{
			CertFile:            certPath,
			KeyFile:             keyPath,
			ES2PlusClientCAFile: caPath,
		},
	})
	if err != nil {
		t.Fatalf("server new: %v", err)
	}

	hs := httptest.NewUnstartedServer(srv.Routes())
	hs.TLS = srv.TLSConfig()
	hs.StartTLS()
	return hs.URL, hs.Close
}

// trustedClient returns an HTTP client that trusts the server cert
// and presents the supplied client cert (if non-nil).
func trustedClient(t *testing.T, pki *labPKI, presentCert *tls.Certificate) *http.Client {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(pki.serverRoot)
	tlsCfg := &tls.Config{RootCAs: roots, ServerName: "127.0.0.1"}
	if presentCert != nil {
		tlsCfg.Certificates = []tls.Certificate{*presentCert}
	}
	return &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
		Timeout:   5 * time.Second,
	}
}

func makeClientTLSCert(t *testing.T, leaf *x509.Certificate, leafKey *ecdsa.PrivateKey) *tls.Certificate {
	t.Helper()
	return &tls.Certificate{
		Certificate: [][]byte{leaf.Raw},
		PrivateKey:  leafKey,
		Leaf:        leaf,
	}
}

// --- tests --------------------------------------------------------------

func TestGateway_TLS_NoMTLS_AcceptsAnyClient(t *testing.T) {
	pki := newLabPKI(t)
	url, stop := startTLSServer(t, pki, false /* mTLS off */)
	defer stop()

	// No client cert presented — should succeed because mTLS isn't enabled.
	resp, err := trustedClient(t, pki, nil).Get(url + "/v1/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestGateway_MTLS_401CounterAdvances(t *testing.T) {
	pki := newLabPKI(t)
	url, stop := startTLSServer(t, pki, true /* mTLS on */)
	defer stop()

	// Drive a few rejected requests on the ES2+ surface — no client cert.
	body := []byte(`{"iccid":"8901234567890123456"}`)
	for i := 0; i < 3; i++ {
		resp, err := trustedClient(t, pki, nil).Post(
			url+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("post %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("post %d: status = %d, want 401", i, resp.StatusCode)
		}
	}

	// /metrics is a non-ES2+ admin path so it stays reachable
	// without a client cert.
	resp, err := trustedClient(t, pki, nil).Get(url + "/metrics")
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	defer resp.Body.Close()
	body2, _ := io.ReadAll(resp.Body)
	got := string(body2)

	// Expected: aether_gateway_es2plus_unauthorized_total{reason="no_client_cert"} 3
	if !strings.Contains(got, `aether_gateway_es2plus_unauthorized_total{reason="no_client_cert"} 3`) {
		t.Fatalf("counter did not advance to 3 for no_client_cert, got:\n%s", got)
	}
	// Other reasons stay at 0.
	for _, want := range []string{
		`aether_gateway_es2plus_unauthorized_total{reason="chain_invalid"} 0`,
		`aether_gateway_es2plus_unauthorized_total{reason="no_tls"} 0`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in /metrics output", want)
		}
	}
}

func TestGateway_MTLS_AdminPathsDoNotRequireClientCert(t *testing.T) {
	pki := newLabPKI(t)
	url, stop := startTLSServer(t, pki, true /* mTLS on */)
	defer stop()

	// /v1/* must work without a client cert even when mTLS is enabled
	// — the operator UI talks here over OIDC, not mTLS.
	resp, err := trustedClient(t, pki, nil).Get(url + "/v1/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin path status = %d, want 200 (mTLS should not gate /v1/*)", resp.StatusCode)
	}
}

func TestGateway_MTLS_ES2PlusRejectsNoClientCert(t *testing.T) {
	pki := newLabPKI(t)
	url, stop := startTLSServer(t, pki, true)
	defer stop()

	body, _ := json.Marshal(map[string]any{"iccid": "8901234567890123456"})
	resp, err := trustedClient(t, pki, nil).Post(
		url+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (no client cert on ES2+)", resp.StatusCode)
	}
}

func TestGateway_MTLS_ES2PlusAcceptsTrustedClientCert(t *testing.T) {
	pki := newLabPKI(t)
	url, stop := startTLSServer(t, pki, true)
	defer stop()

	cert := makeClientTLSCert(t, pki.clientCert, pki.clientCertK)
	body, _ := json.Marshal(map[string]any{"iccid": "8901234567890123456"})
	resp, err := trustedClient(t, pki, cert).Post(
		url+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (valid client cert)", resp.StatusCode)
	}
}

func TestGateway_MTLS_ES2PlusRejectsUntrustedClientCert(t *testing.T) {
	pki := newLabPKI(t)
	url, stop := startTLSServer(t, pki, true)
	defer stop()

	// The "rogue" client cert is signed by an unrelated CA the
	// gateway doesn't trust. The TLS handshake will try to send
	// it, and the server's VerifyClientCertIfGiven will reject it.
	cert := makeClientTLSCert(t, pki.otherClient, pki.otherClientK)
	body, _ := json.Marshal(map[string]any{"iccid": "8901234567890123456"})
	resp, err := trustedClient(t, pki, cert).Post(
		url+"/gsma/rsp2/es2plus/downloadOrder", "application/json", bytes.NewReader(body))
	// Either: Go's TLS handshake fails outright (err != nil), or the
	// connection is accepted but the per-request middleware rejects
	// it with 401. Both are correct rejections.
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("untrusted client cert was accepted (status=200) — gate is not enforcing")
		}
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 or TLS handshake failure", resp.StatusCode)
		}
	}
}

func TestGateway_TLSConfig_RejectsMissingKey(t *testing.T) {
	dir := t.TempDir()
	pki := newLabPKI(t)
	certPath := writePEM(t, dir, "server.crt", pki.serverCert.Raw)
	_, err := New(Config{
		TLS: tlsconf.Config{CertFile: certPath, KeyFile: "/nonexistent"},
	})
	if err == nil {
		t.Fatal("expected error when key file is missing")
	}
}

func TestGateway_TLSConfig_RejectsEmptyClientCAFile(t *testing.T) {
	dir := t.TempDir()
	pki := newLabPKI(t)
	certPath := writePEM(t, dir, "server.crt", pki.serverCert.Raw)
	keyPath := writeKeyPEM(t, dir, "server.key", pki.serverCertK)
	emptyCA := filepath.Join(dir, "empty.pem")
	if err := os.WriteFile(emptyCA, []byte("# no certs here\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := New(Config{
		TLS: tlsconf.Config{CertFile: certPath, KeyFile: keyPath, ES2PlusClientCAFile: emptyCA},
	})
	if err == nil {
		t.Fatal("expected error when client-CA bundle is empty")
	}
}
