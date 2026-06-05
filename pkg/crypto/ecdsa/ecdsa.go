// Package ecdsa wraps Go's standard ECDSA primitives with the curve
// choices and signature encoding that SGP.22 mandates.
//
// SGP.22 §2.6.1 requires support for both NIST P-256 and Brainpool
// P-256 r1 in any compliant implementation. P-256 is provided by the
// Go standard library; Brainpool P-256 r1 comes from
// pkg/crypto/brainpool, which supplies the curve arithmetic the
// standard library omits. See that package's security note: its
// math/big arithmetic is not constant-time, so production Brainpool
// signing belongs in an HSM via services/hsm-broker.
package ecdsa

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/ajamous/aether/pkg/crypto/brainpool"
)

// Curve names the curves SGP.22 §2.6.1 mandates support for.
type Curve int

const (
	// CurveP256 is NIST P-256, also known as secp256r1 / prime256v1.
	CurveP256 Curve = iota
	// CurveBrainpoolP256r1 is the Brainpool P-256 r1 curve, mandated by
	// SGP.22 §2.6.1 and provided via pkg/crypto/brainpool.
	CurveBrainpoolP256r1
)

// SignatureDER is an ECDSA signature in the DER-encoded SEQUENCE { r, s }
// form. SGP.22 §H.5 carries signatures over ASN.1, so DER is the
// natural wire shape for our consumers.
type SignatureDER []byte

// asn1Sig mirrors the standard ECDSA DER signature shape.
type asn1Sig struct {
	R, S *big.Int
}

// GenerateKey generates a new ECDSA key on the requested curve.
//
// Implements the curve selection requirement from SGP.22 §2.6.1.
func GenerateKey(c Curve, rng io.Reader) (*ecdsa.PrivateKey, error) {
	curve, err := goCurve(c)
	if err != nil {
		return nil, err
	}
	if rng == nil {
		rng = rand.Reader
	}
	return ecdsa.GenerateKey(curve, rng)
}

// SignSHA256 signs the SHA-256 digest of msg with priv and returns a
// DER-encoded signature.
//
// SGP.22 §H.5 specifies SHA-256 with both P-256 and Brainpool P-256 r1.
func SignSHA256(priv *ecdsa.PrivateKey, msg []byte) (SignatureDER, error) {
	if priv == nil {
		return nil, errors.New("ecdsa: nil private key")
	}
	digest := sha256.Sum256(msg)
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		return nil, fmt.Errorf("ecdsa: sign: %w", err)
	}
	der, err := asn1.Marshal(asn1Sig{R: r, S: s})
	if err != nil {
		return nil, fmt.Errorf("ecdsa: marshal signature: %w", err)
	}
	return der, nil
}

// VerifySHA256 verifies a DER-encoded ECDSA signature over the SHA-256
// digest of msg.
func VerifySHA256(pub *ecdsa.PublicKey, msg []byte, sig SignatureDER) error {
	if pub == nil {
		return errors.New("ecdsa: nil public key")
	}
	var parsed asn1Sig
	rest, err := asn1.Unmarshal(sig, &parsed)
	if err != nil {
		return fmt.Errorf("ecdsa: unmarshal signature: %w", err)
	}
	if len(rest) != 0 {
		return errors.New("ecdsa: trailing bytes in signature")
	}
	if parsed.R == nil || parsed.S == nil {
		return errors.New("ecdsa: signature missing r or s")
	}
	digest := sha256.Sum256(msg)
	if !ecdsa.Verify(pub, digest[:], parsed.R, parsed.S) {
		return errors.New("ecdsa: signature does not verify")
	}
	return nil
}

func goCurve(c Curve) (elliptic.Curve, error) {
	switch c {
	case CurveP256:
		return elliptic.P256(), nil
	case CurveBrainpoolP256r1:
		return brainpool.P256r1(), nil
	default:
		return nil, fmt.Errorf("ecdsa: unknown curve %d", c)
	}
}
