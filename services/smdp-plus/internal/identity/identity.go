// Package identity provisions and holds the SM-DP+'s DPauth identity:
// the HSM-resident private key handle and the X.509 certificate that
// wraps the corresponding public key.
//
// Two modes:
//
//   - Lab mode: on startup, ask the HSM broker to generate a fresh
//     DPauth keypair and mint a self-signed certificate around the
//     returned public key. The cert won't chain to a CI root, so a
//     real LPA verifying against GSMA roots will reject it. That is
//     deliberate: this mode demonstrates the signing pipeline end to
//     end without a key ceremony, and the cert/CI integration is the
//     next focused PR.
//
//   - Production mode: the operator has run a key ceremony and loaded
//     the DPauth key into the production HSM. The HSM broker exposes
//     it under a known label. The certmgr already serves the
//     CI-issued DPauth certificate. This mode wires those together
//     by referencing them by label/path.
//
// SGP.22 §H.5 sets the signature shape (ECDSA SHA-256 over the DER
// encoding of the message). The actual ECDSA call is delegated to
// the HSM broker via pkg/hsmclient.
package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ajamous/aether/pkg/hsmclient"
)

// Identity holds the references the smdp-plus signing pipeline needs.
type Identity struct {
	// KeyID is the broker's opaque handle to the DPauth private key.
	KeyID string
	// Label is the human-friendly name (e.g. "DPauth").
	Label string
	// PublicKey is the uncompressed X9.63 EC point of the DPauth key.
	PublicKey []byte
	// CertDER is the DER-encoded X.509 certificate wrapping the
	// public key. In lab mode this is self-signed; in production
	// it's the CI-issued cert loaded from disk by certmgr.
	CertDER []byte
	// CertPEM is the same cert in PEM form, for log lines and the UI.
	CertPEM []byte
}

// EnsureLabIdentity provisions a fresh DPauth keypair in the HSM
// broker and mints a self-signed cert around it. Each call generates
// a new key — there is intentionally no persistence in lab mode, so
// the cert and key correspond exactly for the lifetime of the
// process.
//
// label is the broker label (typically "DPauth"). serverDNS is the
// SAN to put on the self-signed cert (typically "aether.local" or
// the operator's lab domain).
func EnsureLabIdentity(ctx context.Context, hc *hsmclient.Client, label, serverDNS string) (*Identity, error) {
	if hc == nil {
		return nil, errors.New("identity: hsm client is required")
	}
	gen, err := hc.GenerateKeyPair(ctx, label, hsmclient.KeyKindECDSA, hsmclient.CurveP256)
	if err != nil {
		return nil, fmt.Errorf("identity: GenerateKeyPair %s: %w", label, err)
	}

	pubKey, err := unmarshalP256Public(gen.PublicKey)
	if err != nil {
		return nil, err
	}

	// Self-sign the cert template using a transient key. We do NOT
	// use this transient key for anything else — it exists only to
	// produce a valid X.509 wrapper around the broker-resident
	// public key. SGP.22 verifiers in production will reject this
	// cert because it doesn't chain to a CI root; that is the
	// honest signal "you're in lab mode."
	signerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("identity: transient signer keygen: %w", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "Aether Lab " + label + " (TEST ONLY)"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		DNSNames:     []string{serverDNS},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pubKey, signerKey)
	if err != nil {
		return nil, fmt.Errorf("identity: CreateCertificate: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	return &Identity{
		KeyID:     gen.Handle.ID,
		Label:     label,
		PublicKey: gen.PublicKey,
		CertDER:   der,
		CertPEM:   pemBytes,
	}, nil
}

// unmarshalP256Public parses the uncompressed X9.63 point the broker
// returns into an ecdsa.PublicKey on P-256. The wire shape is
// `0x04 || X(32) || Y(32)` per SEC1 §2.3.3.
func unmarshalP256Public(point []byte) (*ecdsa.PublicKey, error) {
	const coordLen = 32
	if len(point) != 1+2*coordLen || point[0] != 0x04 {
		return nil, errors.New("identity: failed to unmarshal P-256 public point")
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(point[1 : 1+coordLen]),
		Y:     new(big.Int).SetBytes(point[1+coordLen:]),
	}, nil
}
