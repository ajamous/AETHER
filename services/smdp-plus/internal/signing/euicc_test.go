package signing

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"strings"
	"testing"
	"time"
)

// fakeEuicc is a tiny harness that mints a CI root → EUM → eUICC
// chain entirely in process so we can drive the verifier with real
// cryptographic material without any external HSM.
type fakeEuicc struct {
	root    *x509.Certificate
	rootKey *ecdsa.PrivateKey

	eum    *x509.Certificate
	eumKey *ecdsa.PrivateKey

	leaf    *x509.Certificate
	leafKey *ecdsa.PrivateKey
}

func newFakeEuicc(t *testing.T) *fakeEuicc {
	t.Helper()
	now := time.Now()
	notAfter := now.Add(24 * time.Hour)

	rootKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rootTpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Lab CI Root"},
		NotBefore:    now, NotAfter: notAfter,
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER, _ := x509.CreateCertificate(rand.Reader, rootTpl, rootTpl, &rootKey.PublicKey, rootKey)
	root, _ := x509.ParseCertificate(rootDER)

	eumKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	eumTpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Lab EUM"},
		NotBefore:    now, NotAfter: notAfter,
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign,
	}
	eumDER, _ := x509.CreateCertificate(rand.Reader, eumTpl, root, &eumKey.PublicKey, rootKey)
	eum, _ := x509.ParseCertificate(eumDER)

	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "Lab eUICC #1"},
		NotBefore:    now, NotAfter: notAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}
	leafDER, _ := x509.CreateCertificate(rand.Reader, leafTpl, eum, &leafKey.PublicKey, eumKey)
	leaf, _ := x509.ParseCertificate(leafDER)

	return &fakeEuicc{
		root: root, rootKey: rootKey,
		eum: eum, eumKey: eumKey,
		leaf: leaf, leafKey: leafKey,
	}
}

// signResponse builds an EuiccSigned1 SEQUENCE with the given fields,
// signs it with the leaf key, and returns the four pieces the LPA
// would forward.
func (f *fakeEuicc) signResponse(t *testing.T, txid, euiccChallenge []byte, serverAddress string, serverChallenge []byte) (signedDER, sig, leafDER, eumDER []byte) {
	t.Helper()
	payload := EuiccSigned1{
		TransactionID:   txid,
		ServerAddress:   serverAddress,
		ServerChallenge: serverChallenge,
		EUICCInfo2:      asn1.RawValue{Tag: asn1.TagOctetString, Bytes: euiccChallenge}, // placeholder
		CtxParams1:      asn1.RawValue{Tag: asn1.TagOctetString, Bytes: []byte{0x00}},   // placeholder
	}
	der, err := payload.MarshalDER()
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	digest := sha256.Sum256(der)
	r, s, err := ecdsa.Sign(rand.Reader, f.leafKey, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sigDER, _ := asn1.Marshal(struct{ R, S *big.Int }{R: r, S: s})
	return der, sigDER, f.leaf.Raw, f.eum.Raw
}

func TestVerify_HappyPath(t *testing.T) {
	e := newFakeEuicc(t)
	roots := x509.NewCertPool()
	roots.AddCert(e.root)

	signedDER, sig, leafDER, eumDER := e.signResponse(t,
		[]byte{0x00, 0x11}, bytes.Repeat([]byte{0xAA}, 16),
		"aether.local", bytes.Repeat([]byte{0xBB}, 16))

	res, err := VerifyEuiccAuthenticate(signedDER, sig, leafDER, eumDER, VerifyOptions{
		Roots:         roots,
		ServerAddress: "aether.local",
	})
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if res.EuiccSigned1.ServerAddress != "aether.local" {
		t.Fatalf("serverAddress mismatch")
	}
	if !bytes.Equal(res.EuiccSigned1.ServerChallenge, bytes.Repeat([]byte{0xBB}, 16)) {
		t.Fatal("serverChallenge mismatch")
	}
}

func TestVerify_RejectsTamperedPayload(t *testing.T) {
	e := newFakeEuicc(t)
	roots := x509.NewCertPool()
	roots.AddCert(e.root)
	signedDER, sig, leaf, eum := e.signResponse(t,
		[]byte{0x01}, bytes.Repeat([]byte{0xAA}, 16),
		"aether.local", bytes.Repeat([]byte{0xBB}, 16))

	signedDER[len(signedDER)-1] ^= 0x01
	if _, err := VerifyEuiccAuthenticate(signedDER, sig, leaf, eum, VerifyOptions{Roots: roots, ServerAddress: "aether.local"}); err == nil {
		t.Fatal("expected verify failure on tampered payload")
	}
}

func TestVerify_RejectsTamperedSignature(t *testing.T) {
	e := newFakeEuicc(t)
	roots := x509.NewCertPool()
	roots.AddCert(e.root)
	signedDER, sig, leaf, eum := e.signResponse(t,
		[]byte{0x02}, bytes.Repeat([]byte{0xAA}, 16),
		"aether.local", bytes.Repeat([]byte{0xBB}, 16))

	sig[len(sig)-1] ^= 0x01
	if _, err := VerifyEuiccAuthenticate(signedDER, sig, leaf, eum, VerifyOptions{Roots: roots, ServerAddress: "aether.local"}); err == nil {
		t.Fatal("expected verify failure on tampered signature")
	}
}

func TestVerify_RejectsUnknownRoot(t *testing.T) {
	e := newFakeEuicc(t)
	signedDER, sig, leaf, eum := e.signResponse(t,
		[]byte{0x03}, bytes.Repeat([]byte{0xAA}, 16),
		"aether.local", bytes.Repeat([]byte{0xBB}, 16))

	// Empty trust store — should reject because chain doesn't terminate.
	_, err := VerifyEuiccAuthenticate(signedDER, sig, leaf, eum, VerifyOptions{
		Roots: x509.NewCertPool(), ServerAddress: "aether.local",
	})
	if err == nil || !strings.Contains(err.Error(), "chain") {
		t.Fatalf("expected chain failure, got %v", err)
	}
}

func TestVerify_RejectsWrongServerAddress(t *testing.T) {
	e := newFakeEuicc(t)
	roots := x509.NewCertPool()
	roots.AddCert(e.root)
	signedDER, sig, leaf, eum := e.signResponse(t,
		[]byte{0x04}, bytes.Repeat([]byte{0xAA}, 16),
		"attacker.example", bytes.Repeat([]byte{0xBB}, 16))

	if _, err := VerifyEuiccAuthenticate(signedDER, sig, leaf, eum, VerifyOptions{
		Roots: roots, ServerAddress: "aether.local",
	}); err == nil {
		t.Fatal("expected error on serverAddress mismatch")
	}
}

func TestUnmarshal_RejectsTrailingBytes(t *testing.T) {
	e := newFakeEuicc(t)
	signedDER, _, _, _ := e.signResponse(t,
		[]byte{0x05}, bytes.Repeat([]byte{0xAA}, 16),
		"aether.local", bytes.Repeat([]byte{0xBB}, 16))
	signedDER = append(signedDER, 0xFF)
	if _, err := UnmarshalEuiccSigned1(signedDER); err == nil {
		t.Fatal("expected error on trailing bytes")
	}
}
