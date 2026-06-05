// Package ecka implements ECKA (Elliptic Curve Key Agreement) for SGP.22.
//
// SGP.22 §2.6.4 specifies ECKA as raw ECDH followed by the X9.63 KDF
// with SHA-256 to derive session key material. The shared secret is
// the X coordinate of the ECDH product; the KDF input includes a
// shared-info string carrying the protocol context.
//
// This package handles the agreement and derivation. It does not
// know about the SGP.22 ES8+ session key labelling; that lives in
// pkg/crypto/bsp.
package ecka

import (
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/ajamous/aether/pkg/crypto/kdf"
)

// Curve names a curve SGP.22 §2.6.1 mandates. Both are supported by the
// curve-agnostic API (Generate / PrivateKey.DeriveBytes). The legacy
// crypto/ecdh-native API on this file (GenerateKeyPair / Derive /
// KeyPair) is P-256 only; it returns ErrBrainpoolNotImplemented for
// Brainpool because crypto/ecdh cannot represent a custom curve. New
// callers that need either curve should use Generate.
type Curve int

const (
	CurveP256 Curve = iota
	CurveBrainpoolP256r1
)

// ErrBrainpoolNotImplemented is returned by the legacy crypto/ecdh-based
// KeyPair API for Brainpool keys. The curve-agnostic Generate /
// PrivateKey API supports Brainpool P-256 r1; prefer it.
var ErrBrainpoolNotImplemented = errors.New("ecka: brainpool P-256 r1 not supported by the crypto/ecdh KeyPair API; use Generate")

// KeyPair holds an ECKA key pair on a SGP.22-supported curve.
type KeyPair struct {
	Priv *ecdh.PrivateKey
	Pub  *ecdh.PublicKey
}

// GenerateKeyPair produces a fresh ECKA key pair on the requested curve.
func GenerateKeyPair(c Curve) (*KeyPair, error) {
	curve, err := goCurve(c)
	if err != nil {
		return nil, err
	}
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ecka: generate: %w", err)
	}
	return &KeyPair{Priv: priv, Pub: priv.PublicKey()}, nil
}

// Derive returns keyLen bytes of derived key material from the ECDH
// product of priv and peerPub, run through X9.63-SHA-256 with sharedInfo
// as defined per the calling protocol (SGP.22 §2.6.4).
func Derive(priv *ecdh.PrivateKey, peerPub *ecdh.PublicKey, sharedInfo []byte, keyLen int) ([]byte, error) {
	if priv == nil || peerPub == nil {
		return nil, errors.New("ecka: nil key")
	}
	z, err := priv.ECDH(peerPub)
	if err != nil {
		return nil, fmt.Errorf("ecka: ECDH: %w", err)
	}
	return kdf.X963SHA256(z, sharedInfo, keyLen)
}

func goCurve(c Curve) (ecdh.Curve, error) {
	switch c {
	case CurveP256:
		return ecdh.P256(), nil
	case CurveBrainpoolP256r1:
		return nil, ErrBrainpoolNotImplemented
	default:
		return nil, fmt.Errorf("ecka: unknown curve %d", c)
	}
}
