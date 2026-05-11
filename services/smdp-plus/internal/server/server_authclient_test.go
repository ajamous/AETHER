package server

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ajamous/aether/pkg/hsmclient"
	smdpv1 "github.com/ajamous/aether/services/smdp-plus/api/v1"
	"github.com/ajamous/aether/services/smdp-plus/internal/identity"
	"github.com/ajamous/aether/services/smdp-plus/internal/session"
	"github.com/ajamous/aether/services/smdp-plus/internal/signing"
)

// labChain mints a CI root → EUM → eUICC chain for tests. The
// returned material lets us drive both:
//   - the SM-DP+ side: trust pool seeded with the root
//   - the synthetic eUICC: signs with the leaf key, presents the
//     leaf and EUM certs in the response
type labChain struct {
	rootCert *x509.Certificate
	rootKey  *ecdsa.PrivateKey
	eumCert  *x509.Certificate
	eumKey   *ecdsa.PrivateKey
	leafCert *x509.Certificate
	leafKey  *ecdsa.PrivateKey
}

func newLabChain(t *testing.T) *labChain {
	t.Helper()
	now := time.Now()
	notAfter := now.Add(24 * time.Hour)

	mk := func(serial int64, subj string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey, isCA bool) (*x509.Certificate, *ecdsa.PrivateKey) {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tpl := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: subj},
			NotBefore:    now, NotAfter: notAfter,
			IsCA: isCA, BasicConstraintsValid: isCA,
		}
		if isCA {
			tpl.KeyUsage = x509.KeyUsageCertSign
		} else {
			tpl.KeyUsage = x509.KeyUsageDigitalSignature
			tpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageAny}
		}
		signer := tpl
		signerKey := key
		if parent != nil {
			signer = parent
			signerKey = parentKey
		}
		der, _ := x509.CreateCertificate(rand.Reader, tpl, signer, &key.PublicKey, signerKey)
		c, _ := x509.ParseCertificate(der)
		return c, key
	}

	root, rootKey := mk(1, "Lab CI Root", nil, nil, true)
	eum, eumKey := mk(2, "Lab EUM", root, rootKey, true)
	leaf, leafKey := mk(3, "Lab eUICC #1", eum, eumKey, false)
	return &labChain{
		rootCert: root, rootKey: rootKey,
		eumCert: eum, eumKey: eumKey,
		leafCert: leaf, leafKey: leafKey,
	}
}

// signAuthenticateResponse produces what the LPA would forward.
func (c *labChain) signAuthenticateResponse(t *testing.T, txid []byte, serverAddress string, serverChallenge []byte) (signedDER, sig, leafDER, eumDER []byte) {
	t.Helper()
	payload := signing.EuiccSigned1{
		TransactionID:   txid,
		ServerAddress:   serverAddress,
		ServerChallenge: serverChallenge,
		EUICCInfo2:      asn1.RawValue{Tag: asn1.TagOctetString, Bytes: []byte{0xAA}},
		CtxParams1:      asn1.RawValue{Tag: asn1.TagOctetString, Bytes: []byte{0xBB}},
	}
	der, err := payload.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	digest := sha256.Sum256(der)
	r, s, err := ecdsa.Sign(rand.Reader, c.leafKey, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sigDER, _ := asn1.Marshal(struct{ R, S *big.Int }{R: r, S: s})
	return der, sigDER, c.leafCert.Raw, c.eumCert.Raw
}

// stand up a fake HSM broker that signs with a real ECDSA key,
// then a smdp-plus server with full signing+verification enabled.
func newAuthcheckSrv(t *testing.T, chain *labChain) (smdpURL string, txid []byte, serverChallenge []byte, cleanup func()) {
	t.Helper()

	// Fake hsm-broker.
	smdpKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecdhPub, _ := smdpKey.PublicKey.ECDH()
	pubPoint := ecdhPub.Bytes()
	brokerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/generate-key-pair"):
			json.NewEncoder(w).Encode(hsmclient.GenerateKeyPairResponse{
				Handle:    hsmclient.KeyHandle{ID: "DPauth-key", Label: "DPauth", Kind: hsmclient.KeyKindECDSA, Curve: hsmclient.CurveP256},
				PublicKey: pubPoint,
			})
		case strings.HasSuffix(r.URL.Path, "/v1/sign"):
			var req struct {
				KeyID  string `json:"key_id"`
				Digest []byte `json:"digest"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			r2, s2, _ := ecdsa.Sign(rand.Reader, smdpKey, req.Digest)
			der, _ := asn1.Marshal(struct{ R, S *big.Int }{R: r2, S: s2})
			json.NewEncoder(w).Encode(hsmclient.SignResponse{SignatureDER: der})
		default:
			http.NotFound(w, r)
		}
	}))

	hc := hsmclient.New(brokerSrv.URL)
	id, err := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	if err != nil {
		t.Fatalf("identity: %v", err)
	}

	// Trust material seeded directly from the lab chain (skips the
	// certmgr fetch — the FetchTrustMaterial path is covered in its
	// own tests).
	roots := x509.NewCertPool()
	roots.AddCert(chain.rootCert)
	tm := &identity.TrustMaterial{
		Roots:         roots,
		Intermediates: x509.NewCertPool(),
	}

	smdpSrv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{HSM: hc, Identity: id, Trust: tm, Address: "aether.local"},
	).Routes())

	// Drive initiateAuthentication so we have a transaction id and the
	// server challenge that euiccSigned1 must echo.
	euiccChallenge := bytes.Repeat([]byte{0xAB}, 16)
	body, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: euiccChallenge,
		SMDPAddress:    "aether.local",
	})
	resp, err := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	var initOut smdpv1.InitiateAuthenticationResponse
	if err := json.NewDecoder(resp.Body).Decode(&initOut); err != nil {
		t.Fatalf("decode init: %v", err)
	}
	resp.Body.Close()

	parsedSigned1, err := signing.UnmarshalServerSigned1(initOut.ServerSigned1)
	if err != nil {
		t.Fatalf("parse server signed1: %v", err)
	}
	tidBytes := parsedSigned1.TransactionID

	cleanup = func() {
		smdpSrv.Close()
		brokerSrv.Close()
	}
	return smdpSrv.URL, tidBytes, parsedSigned1.ServerChallenge, cleanup
}

func TestAuthenticateClient_VerifiesGoodResponse(t *testing.T) {
	chain := newLabChain(t)
	url, txid, serverChallenge, cleanup := newAuthcheckSrv(t, chain)
	defer cleanup()

	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, txid, "aether.local", serverChallenge)

	body, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(txid),
		EuiccSigned1DER: signed,
		EuiccSignature1: sig,
		EuiccCertDER:    leafDER,
		EumCertDER:      eumDER,
	})
	resp, err := http.Post(url+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var prob map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&prob)
		t.Fatalf("status = %d, body = %v", resp.StatusCode, prob)
	}
}

func TestAuthenticateClient_RejectsWrongServerChallenge(t *testing.T) {
	chain := newLabChain(t)
	url, txid, _, cleanup := newAuthcheckSrv(t, chain)
	defer cleanup()

	wrongChallenge := bytes.Repeat([]byte{0xFF}, 16)
	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, txid, "aether.local", wrongChallenge)

	body, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(txid),
		EuiccSigned1DER: signed,
		EuiccSignature1: sig,
		EuiccCertDER:    leafDER,
		EumCertDER:      eumDER,
	})
	resp, err := http.Post(url+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 — must reject replay", resp.StatusCode)
	}
}

func TestAuthenticateClient_RejectsEUICCFromUnknownCI(t *testing.T) {
	chain := newLabChain(t)
	url, txid, serverChallenge, cleanup := newAuthcheckSrv(t, chain)
	defer cleanup()

	// A second, unrelated chain. Its leaf cannot verify against the
	// SM-DP+'s trust store (which holds only the first chain's root).
	other := newLabChain(t)
	signed, sig, leafDER, eumDER := other.signAuthenticateResponse(t, txid, "aether.local", serverChallenge)

	body, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(txid),
		EuiccSigned1DER: signed,
		EuiccSignature1: sig,
		EuiccCertDER:    leafDER,
		EumCertDER:      eumDER,
	})
	resp, err := http.Post(url+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuthenticateClient_RejectsTamperedSignature(t *testing.T) {
	chain := newLabChain(t)
	url, txid, serverChallenge, cleanup := newAuthcheckSrv(t, chain)
	defer cleanup()

	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, txid, "aether.local", serverChallenge)
	sig[len(sig)-1] ^= 0x01

	body, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(txid),
		EuiccSigned1DER: signed,
		EuiccSignature1: sig,
		EuiccCertDER:    leafDER,
		EumCertDER:      eumDER,
	})
	resp, err := http.Post(url+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAuthenticateClient_MissingFieldsRejected(t *testing.T) {
	chain := newLabChain(t)
	url, txid, _, cleanup := newAuthcheckSrv(t, chain)
	defer cleanup()

	body, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID: hexEncode(txid),
	})
	resp, err := http.Post(url+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func hexEncode(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0xF]
	}
	return string(out)
}

// TestAuthenticateClient_DPpbSigningEndToEnd configures the server
// with both DPauth and DPpb identities, drives a full
// initiateAuthentication + authenticateClient flow with a real
// eUICC chain, and verifies that the returned SmdpSigned2 +
// signature decode and ECDSA-verify against the broker's public
// key. Mirrors TestSignSmdpSigned2_VerifiesEndToEnd from the
// signing package but at the HTTP-handler level so we know the
// full plumbing works.
//
// SAS-SM-relevant: a real eUICC will reject the
// authenticateClient response if SmdpSigned2 doesn't verify
// against the SM-DP+'s DPpb cert chain. This test is the
// closest thing in CI to that check until the hardware bench
// lands.
func TestAuthenticateClient_DPpbSigningEndToEnd(t *testing.T) {
	chain := newLabChain(t)

	// Fake hsm-broker that signs any digest with one ECDSA key.
	smdpKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecdhPub, _ := smdpKey.PublicKey.ECDH()
	pubPoint := ecdhPub.Bytes()
	brokerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/generate-key-pair"):
			json.NewEncoder(w).Encode(hsmclient.GenerateKeyPairResponse{
				Handle:    hsmclient.KeyHandle{ID: "key", Label: "key", Kind: hsmclient.KeyKindECDSA, Curve: hsmclient.CurveP256},
				PublicKey: pubPoint,
			})
		case strings.HasSuffix(r.URL.Path, "/v1/sign"):
			var req struct {
				KeyID  string `json:"key_id"`
				Digest []byte `json:"digest"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			r2, s2, _ := ecdsa.Sign(rand.Reader, smdpKey, req.Digest)
			der, _ := asn1.Marshal(struct{ R, S *big.Int }{R: r2, S: s2})
			json.NewEncoder(w).Encode(hsmclient.SignResponse{SignatureDER: der})
		default:
			http.NotFound(w, r)
		}
	}))
	defer brokerSrv.Close()
	hc := hsmclient.New(brokerSrv.URL)

	dpauth, err := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	if err != nil {
		t.Fatalf("dpauth: %v", err)
	}
	dppb, err := identity.EnsureLabIdentity(context.Background(), hc, "DPpb", "aether.local")
	if err != nil {
		t.Fatalf("dppb: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(chain.rootCert)
	tm := &identity.TrustMaterial{Roots: roots, Intermediates: x509.NewCertPool()}

	smdpSrv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{
			HSM:      hc,
			Identity: dpauth,
			DPpb:     dppb,
			Trust:    tm,
			Address:  "aether.local",
		},
	).Routes())
	defer smdpSrv.Close()

	// Drive initiateAuthentication.
	euiccChallenge := bytes.Repeat([]byte{0xAB}, 16)
	body, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: euiccChallenge,
		SMDPAddress:    "aether.local",
	})
	resp, err := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("initiate post: %v", err)
	}
	var initOut smdpv1.InitiateAuthenticationResponse
	json.NewDecoder(resp.Body).Decode(&initOut)
	resp.Body.Close()
	parsedSigned1, _ := signing.UnmarshalServerSigned1(initOut.ServerSigned1)
	tidBytes := parsedSigned1.TransactionID
	serverChallenge := parsedSigned1.ServerChallenge

	// Now authenticateClient.
	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, tidBytes, "aether.local", serverChallenge)
	authBody, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(tidBytes),
		EuiccSigned1DER: signed,
		EuiccSignature1: sig,
		EuiccCertDER:    leafDER,
		EumCertDER:      eumDER,
	})
	resp, err = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(authBody))
	if err != nil {
		t.Fatalf("authenticateClient: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var prob map[string]any
		json.NewDecoder(resp.Body).Decode(&prob)
		t.Fatalf("status = %d, body = %v", resp.StatusCode, prob)
	}

	var out smdpv1.AuthenticateClientResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.SMDPSigned2) == 0 {
		t.Fatal("SMDPSigned2 empty — DPpb signing did not run")
	}
	if len(out.SMDPSignature2) == 0 {
		t.Fatal("SMDPSignature2 empty")
	}
	if len(out.SMDPCertificate) == 0 {
		t.Fatal("SMDPCertificate empty")
	}

	// Decode the signed payload and confirm the transactionId
	// matches what we issued — the spec invariant the eUICC
	// checks before trusting the upcoming BPP.
	parsed, err := signing.UnmarshalSmdpSigned2(out.SMDPSigned2)
	if err != nil {
		t.Fatalf("unmarshal SmdpSigned2: %v", err)
	}
	if !bytes.Equal(parsed.TransactionID, tidBytes) {
		t.Errorf("SmdpSigned2.transactionId = %x, want %x", parsed.TransactionID, tidBytes)
	}

	// Verify the ECDSA signature against the broker's public key.
	digest := sha256.Sum256(out.SMDPSigned2)
	var ecdsaSig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(out.SMDPSignature2, &ecdsaSig); err != nil {
		t.Fatalf("sig unmarshal: %v", err)
	}
	if !ecdsa.Verify(&smdpKey.PublicKey, digest[:], ecdsaSig.R, ecdsaSig.S) {
		t.Fatal("ECDSA verify failed against the broker's public key — eUICC would reject this BPP")
	}
}

// TestAuthenticateClient_NoDPpbLeavesSmdpSigned2Empty confirms the
// lab path stays unchanged — when DPpb isn't configured, the
// response carries no SmdpSigned2 fields and existing tests
// (TestAuthenticateClient_VerifiesGoodResponse and friends)
// continue to pass on the same setup.
func TestAuthenticateClient_NoDPpbLeavesSmdpSigned2Empty(t *testing.T) {
	chain := newLabChain(t)
	url, txid, serverChallenge, cleanup := newAuthcheckSrv(t, chain)
	defer cleanup()

	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, txid, "aether.local", serverChallenge)
	body, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(txid),
		EuiccSigned1DER: signed,
		EuiccSignature1: sig,
		EuiccCertDER:    leafDER,
		EumCertDER:      eumDER,
	})
	resp, err := http.Post(url+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("authenticateClient post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out smdpv1.AuthenticateClientResponse
	json.NewDecoder(resp.Body).Decode(&out)
	if len(out.SMDPSigned2) != 0 || len(out.SMDPSignature2) != 0 || len(out.SMDPCertificate) != 0 {
		t.Errorf("DPpb-not-configured path must leave SmdpSigned2 fields empty; got SMDPSigned2=%d SMDPSignature2=%d SMDPCertificate=%d",
			len(out.SMDPSigned2), len(out.SMDPSignature2), len(out.SMDPCertificate))
	}
}

// TestGetBoundProfilePackage_HappyPath drives a full SGP.22
// initiate → authenticate → getBPP flow with DPpb wired and an
// eUICC ephemeral pubkey supplied directly. Verifies the
// returned BoundProfilePackage:
//
//   - decodes from the JSON response as non-empty DER bytes
//   - starts with the [APPLICATION 54] constructed high-tag-form
//     opening (matches bpp.AssembleBoundProfilePackage's outer wrap)
//
// As close to "an eUICC would accept this BPP" as we get without a
// hardware bench. Cross-vendor interop is the named follow-up.
func TestGetBoundProfilePackage_HappyPath(t *testing.T) {
	chain := newLabChain(t)

	smdpKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecdhPub, _ := smdpKey.PublicKey.ECDH()
	pubPoint := ecdhPub.Bytes()
	brokerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/generate-key-pair"):
			json.NewEncoder(w).Encode(hsmclient.GenerateKeyPairResponse{
				Handle:    hsmclient.KeyHandle{ID: "key", Label: "key", Kind: hsmclient.KeyKindECDSA, Curve: hsmclient.CurveP256},
				PublicKey: pubPoint,
			})
		case strings.HasSuffix(r.URL.Path, "/v1/sign"):
			var req struct {
				KeyID  string `json:"key_id"`
				Digest []byte `json:"digest"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			r2, s2, _ := ecdsa.Sign(rand.Reader, smdpKey, req.Digest)
			der, _ := asn1.Marshal(struct{ R, S *big.Int }{R: r2, S: s2})
			json.NewEncoder(w).Encode(hsmclient.SignResponse{SignatureDER: der})
		default:
			http.NotFound(w, r)
		}
	}))
	defer brokerSrv.Close()
	hc := hsmclient.New(brokerSrv.URL)

	dpauth, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	dppb, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPpb", "aether.local")

	roots := x509.NewCertPool()
	roots.AddCert(chain.rootCert)
	tm := &identity.TrustMaterial{Roots: roots, Intermediates: x509.NewCertPool()}

	smdpSrv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{HSM: hc, Identity: dpauth, DPpb: dppb, Trust: tm, Address: "aether.local"},
	).Routes())
	defer smdpSrv.Close()

	// initiateAuthentication.
	euiccChallenge := bytes.Repeat([]byte{0xAB}, 16)
	body, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: euiccChallenge,
		SMDPAddress:    "aether.local",
	})
	resp, err := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("initiate post: %v", err)
	}
	var initOut smdpv1.InitiateAuthenticationResponse
	json.NewDecoder(resp.Body).Decode(&initOut)
	resp.Body.Close()
	parsedSigned1, _ := signing.UnmarshalServerSigned1(initOut.ServerSigned1)
	tidBytes := parsedSigned1.TransactionID
	serverChallenge := parsedSigned1.ServerChallenge

	// authenticateClient.
	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, tidBytes, "aether.local", serverChallenge)
	authBody, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(tidBytes),
		EuiccSigned1DER: signed,
		EuiccSignature1: sig,
		EuiccCertDER:    leafDER,
		EumCertDER:      eumDER,
	})
	resp, err = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(authBody))
	if err != nil {
		t.Fatalf("authenticateClient: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		var prob map[string]any
		json.NewDecoder(resp.Body).Decode(&prob)
		t.Fatalf("authenticateClient status = %d, body = %v", resp.StatusCode, prob)
	}
	resp.Body.Close()

	// Generate a real eUICC ephemeral pubkey to feed in.
	euiccEphemeral, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("euicc ephemeral keygen: %v", err)
	}
	euiccOtpk := euiccEphemeral.PublicKey().Bytes() // uncompressed X9.63 point

	// getBoundProfilePackage with the eUICC otPK supplied directly.
	bppBody, _ := json.Marshal(smdpv1.GetBoundProfilePackageRequest{
		TransactionID: hexEncode(tidBytes),
		EUICCOtpk:     euiccOtpk,
	})
	resp, err = http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/getBoundProfilePackage", "application/json", bytes.NewReader(bppBody))
	if err != nil {
		t.Fatalf("getBPP: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var prob map[string]any
		json.NewDecoder(resp.Body).Decode(&prob)
		t.Fatalf("getBPP status = %d (want 200), body = %v", resp.StatusCode, prob)
	}

	var out smdpv1.GetBoundProfilePackageResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.TransactionID != hexEncode(tidBytes) {
		t.Errorf("transactionId echo = %q, want %q", out.TransactionID, hexEncode(tidBytes))
	}
	if len(out.BoundProfilePackage) == 0 {
		t.Fatal("bound_profile_package empty — handler did not produce a BPP")
	}
	// Outer tag is [APPLICATION 54] constructed high-tag-form:
	// 0x7F (APPLICATION + constructed + high-tag) followed by 0x36 (= 54).
	if out.BoundProfilePackage[0] != 0x7F {
		t.Errorf("BPP first byte = 0x%02x, want 0x7F (APPLICATION constructed)", out.BoundProfilePackage[0])
	}
	if len(out.BoundProfilePackage) > 1 && out.BoundProfilePackage[1] != 0x36 {
		t.Errorf("BPP second byte = 0x%02x, want 0x36 (tag VLQ for 54)", out.BoundProfilePackage[1])
	}
}

// TestGetBoundProfilePackage_RejectsMissingEuiccOtpk confirms the
// new wired path validates inputs strictly: missing or
// malformed eUICC otPK returns 400, not a half-built BPP.
func TestGetBoundProfilePackage_RejectsMissingEuiccOtpk(t *testing.T) {
	chain := newLabChain(t)
	url, txid, serverChallenge, cleanup := newAuthcheckSrvWithDPpb(t, chain)
	defer cleanup()

	// Drive authenticateClient so the session is in `authenticated`.
	signed, sig, leafDER, eumDER := chain.signAuthenticateResponse(t, txid, "aether.local", serverChallenge)
	authBody, _ := json.Marshal(smdpv1.AuthenticateClientRequest{
		TransactionID:   hexEncode(txid),
		EuiccSigned1DER: signed,
		EuiccSignature1: sig,
		EuiccCertDER:    leafDER,
		EumCertDER:      eumDER,
	})
	resp, err := http.Post(url+"/gsma/rsp2/es9plus/authenticateClient", "application/json", bytes.NewReader(authBody))
	if err != nil {
		t.Fatalf("authenticateClient post: %v", err)
	}
	resp.Body.Close()

	// Missing euicc_otpk.
	body, _ := json.Marshal(smdpv1.GetBoundProfilePackageRequest{TransactionID: hexEncode(txid)})
	resp, err = http.Post(url+"/gsma/rsp2/es9plus/getBoundProfilePackage", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("getBoundProfilePackage post (missing otpk): %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing otpk status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Malformed (wrong length).
	body, _ = json.Marshal(smdpv1.GetBoundProfilePackageRequest{
		TransactionID: hexEncode(txid),
		EUICCOtpk:     []byte{0x04, 0x01, 0x02},
	})
	resp, err = http.Post(url+"/gsma/rsp2/es9plus/getBoundProfilePackage", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("getBoundProfilePackage post (malformed otpk): %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed otpk status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()

	// Right length but wrong first byte (compressed-form prefix
	// where uncompressed expected).
	bad := append([]byte{0x02}, bytes.Repeat([]byte{0x01}, 64)...)
	body, _ = json.Marshal(smdpv1.GetBoundProfilePackageRequest{
		TransactionID: hexEncode(txid),
		EUICCOtpk:     bad,
	})
	resp, err = http.Post(url+"/gsma/rsp2/es9plus/getBoundProfilePackage", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("getBoundProfilePackage post (wrong first byte): %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong-first-byte otpk status = %d, want 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// newAuthcheckSrvWithDPpb is the DPpb-enabled variant of
// newAuthcheckSrv. Builds a smdp-plus server with both DPauth +
// DPpb identities, drives initiateAuthentication, and returns the
// (URL, txid, serverChallenge, cleanup) tuple the caller needs to
// finish the flow.
func newAuthcheckSrvWithDPpb(t *testing.T, chain *labChain) (smdpURL string, txid []byte, serverChallenge []byte, cleanup func()) {
	t.Helper()

	smdpKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecdhPub, _ := smdpKey.PublicKey.ECDH()
	pubPoint := ecdhPub.Bytes()
	brokerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/generate-key-pair"):
			json.NewEncoder(w).Encode(hsmclient.GenerateKeyPairResponse{
				Handle:    hsmclient.KeyHandle{ID: "key", Label: "key", Kind: hsmclient.KeyKindECDSA, Curve: hsmclient.CurveP256},
				PublicKey: pubPoint,
			})
		case strings.HasSuffix(r.URL.Path, "/v1/sign"):
			var req struct {
				KeyID  string `json:"key_id"`
				Digest []byte `json:"digest"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			r2, s2, _ := ecdsa.Sign(rand.Reader, smdpKey, req.Digest)
			der, _ := asn1.Marshal(struct{ R, S *big.Int }{R: r2, S: s2})
			json.NewEncoder(w).Encode(hsmclient.SignResponse{SignatureDER: der})
		default:
			http.NotFound(w, r)
		}
	}))
	hc := hsmclient.New(brokerSrv.URL)

	dpauth, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	dppb, _ := identity.EnsureLabIdentity(context.Background(), hc, "DPpb", "aether.local")

	roots := x509.NewCertPool()
	roots.AddCert(chain.rootCert)
	tm := &identity.TrustMaterial{Roots: roots, Intermediates: x509.NewCertPool()}

	smdpSrv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{HSM: hc, Identity: dpauth, DPpb: dppb, Trust: tm, Address: "aether.local"},
	).Routes())

	euiccChallenge := bytes.Repeat([]byte{0xAB}, 16)
	body, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: euiccChallenge,
		SMDPAddress:    "aether.local",
	})
	resp, err := http.Post(smdpSrv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("initiate post: %v", err)
	}
	var initOut smdpv1.InitiateAuthenticationResponse
	json.NewDecoder(resp.Body).Decode(&initOut)
	resp.Body.Close()
	parsedSigned1, _ := signing.UnmarshalServerSigned1(initOut.ServerSigned1)
	cleanup = func() {
		smdpSrv.Close()
		brokerSrv.Close()
	}
	return smdpSrv.URL, parsedSigned1.TransactionID, parsedSigned1.ServerChallenge, cleanup
}
