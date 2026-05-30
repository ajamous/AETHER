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

// signLabEuicc generates a self-signed P-256 leaf cert + an
// EuiccSigned2 + signature, returning everything wired so the verifier
// has a complete blob to chew on.
func signLabEuicc(t *testing.T, txid, otpk []byte) (certDER, blob []byte, priv *ecdsa.PrivateKey) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		Subject:               pkixCommonName("eUICC test"),
		BasicConstraintsValid: true,
	}
	certDER, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("self-sign: %v", err)
	}
	signed := EuiccSigned2{TransactionID: txid, EuiccOtpk: otpk}
	signedDER, err := signed.MarshalDER()
	if err != nil {
		t.Fatalf("marshal signed: %v", err)
	}
	digest := sha256.Sum256(signedDER)
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sigDER, _ := asn1.Marshal(struct{ R, S *big.Int }{r, s})
	blob, err = MarshalPrepareDownloadResponseOk(signed, sigDER)
	if err != nil {
		t.Fatalf("marshal wrapper: %v", err)
	}
	return certDER, blob, priv
}

func pkixCommonName(cn string) pkix.Name { return pkix.Name{CommonName: cn} }

func makeOTPK(t *testing.T) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("otpk keygen: %v", err)
	}
	out := make([]byte, 65)
	out[0] = 0x04
	xBytes := priv.PublicKey.X.Bytes()
	yBytes := priv.PublicKey.Y.Bytes()
	copy(out[33-len(xBytes):33], xBytes)
	copy(out[65-len(yBytes):65], yBytes)
	return out
}

func TestVerifyPrepareDownloadResponse_HappyPath(t *testing.T) {
	txid := bytes.Repeat([]byte{0xAB}, 8)
	otpk := makeOTPK(t)
	certDER, blob, _ := signLabEuicc(t, txid, otpk)

	got, err := VerifyPrepareDownloadResponse(blob, certDER, txid)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !bytes.Equal(got.EuiccOtpk, otpk) {
		t.Errorf("otpk mismatch")
	}
	if !bytes.Equal(got.TransactionID, txid) {
		t.Errorf("txid mismatch")
	}
}

func TestVerifyPrepareDownloadResponse_RejectsWrongTransactionID(t *testing.T) {
	txid := bytes.Repeat([]byte{0xAB}, 8)
	other := bytes.Repeat([]byte{0xCD}, 8)
	certDER, blob, _ := signLabEuicc(t, txid, makeOTPK(t))
	_, err := VerifyPrepareDownloadResponse(blob, certDER, other)
	if err == nil || !strings.Contains(err.Error(), "transactionId") {
		t.Fatalf("expected transactionId error, got %v", err)
	}
}

func TestVerifyPrepareDownloadResponse_RejectsTamperedSignature(t *testing.T) {
	txid := bytes.Repeat([]byte{0xAB}, 8)
	certDER, blob, _ := signLabEuicc(t, txid, makeOTPK(t))
	// Flip the last byte (inside the signature region) and confirm
	// verification fails.
	tampered := append([]byte(nil), blob...)
	tampered[len(tampered)-1] ^= 0xFF
	_, err := VerifyPrepareDownloadResponse(tampered, certDER, txid)
	if err == nil {
		t.Fatal("expected verify error on tampered blob")
	}
}

func TestVerifyPrepareDownloadResponse_RejectsWrongCert(t *testing.T) {
	txid := bytes.Repeat([]byte{0xAB}, 8)
	_, blob, _ := signLabEuicc(t, txid, makeOTPK(t))
	// Use a fresh, unrelated cert as the session cert — verification
	// must fail.
	otherCertDER, _, _ := signLabEuicc(t, txid, makeOTPK(t))
	_, err := VerifyPrepareDownloadResponse(blob, otherCertDER, txid)
	if err == nil || !strings.Contains(err.Error(), "euiccSignature2") {
		t.Fatalf("expected signature mismatch error, got %v", err)
	}
}

func TestVerifyPrepareDownloadResponse_RejectsEmptyCert(t *testing.T) {
	if _, err := VerifyPrepareDownloadResponse([]byte{0x30, 0x00}, nil, []byte{0x01}); err == nil {
		t.Fatal("expected error when session has no captured cert")
	}
}

func TestEuiccSigned2_Roundtrip(t *testing.T) {
	in := EuiccSigned2{
		TransactionID: bytes.Repeat([]byte{0x12}, 4),
		EuiccOtpk:     makeOTPK(t),
	}
	der, err := in.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := UnmarshalEuiccSigned2(der)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !bytes.Equal(out.TransactionID, in.TransactionID) || !bytes.Equal(out.EuiccOtpk, in.EuiccOtpk) {
		t.Errorf("mismatch: %+v", out)
	}
}

func TestEuiccSigned2_Validation(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*EuiccSigned2)
	}{
		{"short txid", func(e *EuiccSigned2) { e.TransactionID = nil }},
		{"long txid", func(e *EuiccSigned2) { e.TransactionID = bytes.Repeat([]byte{0}, 17) }},
		{"otpk wrong length", func(e *EuiccSigned2) { e.EuiccOtpk = []byte{0x04, 0x01} }},
		{"uncompressed bad prefix", func(e *EuiccSigned2) { e.EuiccOtpk[0] = 0x05 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := EuiccSigned2{TransactionID: []byte{1, 2, 3}, EuiccOtpk: makeOTPK(t)}
			c.mut(&e)
			if _, err := e.MarshalDER(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}
