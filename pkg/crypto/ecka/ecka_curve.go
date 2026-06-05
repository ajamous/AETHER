package ecka

import (
	"crypto/ecdh"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/ajamous/aether/pkg/crypto/brainpool"
	"github.com/ajamous/aether/pkg/crypto/kdf"
)

// PrivateKey is an ECKA private key that abstracts over the curve
// backend so a single call site can agree keys on either curve SGP.22
// §2.6.1 mandates:
//
//   - CurveP256 runs on crypto/ecdh's constant-time implementation.
//   - CurveBrainpoolP256r1 runs on pkg/crypto/brainpool's math/big
//     arithmetic, which is NOT constant-time. See that package's
//     security note: a long-lived Brainpool agreement key on an
//     untrusted host should live in an HSM, not here.
//
// This is the curve-agnostic counterpart to KeyPair, which remains
// the P-256-only, crypto/ecdh-native type used by the consumer BPP
// session-key path (pkg/crypto/bsp, services/smdp-plus).
type PrivateKey struct {
	curve Curve
	p256  *ecdh.PrivateKey // populated iff curve == CurveP256
	bp    *bpPrivate       // populated iff curve == CurveBrainpoolP256r1
}

type bpPrivate struct {
	d    *big.Int
	x, y *big.Int
}

// brainpoolByteLen is the coordinate width of brainpoolP256r1.
const brainpoolByteLen = 32

// Generate produces a fresh ECKA private key on the requested curve.
func Generate(c Curve) (*PrivateKey, error) {
	switch c {
	case CurveP256:
		k, err := ecdh.P256().GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("ecka: generate P-256: %w", err)
		}
		return &PrivateKey{curve: c, p256: k}, nil
	case CurveBrainpoolP256r1:
		d, err := randScalar(brainpool.Params().N)
		if err != nil {
			return nil, fmt.Errorf("ecka: generate Brainpool: %w", err)
		}
		x, y := brainpool.ScalarBaseMult(d.Bytes())
		return &PrivateKey{curve: c, bp: &bpPrivate{d: d, x: x, y: y}}, nil
	default:
		return nil, fmt.Errorf("ecka: unknown curve %d", c)
	}
}

// Curve reports which curve this key lives on.
func (k *PrivateKey) Curve() Curve { return k.curve }

// PublicBytes returns the public key as an uncompressed X9.63 point
// (SEC1 §2.3.3: 0x04 || X || Y, each coordinate left-padded to the
// curve's byte length) — the wire form SGP.22 carries on ES8+/ES9+.
func (k *PrivateKey) PublicBytes() []byte {
	switch k.curve {
	case CurveP256:
		return k.p256.PublicKey().Bytes()
	case CurveBrainpoolP256r1:
		return marshalUncompressed(k.bp.x, k.bp.y)
	default:
		return nil
	}
}

// DeriveBytes performs ECKA (raw ECDH then X9.63-SHA-256, SGP.22
// §2.6.4) against a peer public key in uncompressed X9.63 form, and
// returns keyLen bytes of derived material. peerPublic is validated:
// it must be a well-formed point on the same curve and must not be the
// point at infinity. brainpoolP256r1 has cofactor 1, so on-curve plus
// non-identity is a complete public-key validation (no small-subgroup
// exposure).
func (k *PrivateKey) DeriveBytes(peerPublic, sharedInfo []byte, keyLen int) ([]byte, error) {
	switch k.curve {
	case CurveP256:
		peer, err := ecdh.P256().NewPublicKey(peerPublic)
		if err != nil {
			return nil, fmt.Errorf("ecka: peer pubkey: %w", err)
		}
		z, err := k.p256.ECDH(peer)
		if err != nil {
			return nil, fmt.Errorf("ecka: ECDH: %w", err)
		}
		return kdf.X963SHA256(z, sharedInfo, keyLen)
	case CurveBrainpoolP256r1:
		px, py, err := parseUncompressed(peerPublic)
		if err != nil {
			return nil, err
		}
		if !brainpool.IsOnCurve(px, py) {
			return nil, errors.New("ecka: peer pubkey is not on the curve")
		}
		zx, zy := brainpool.ScalarMult(px, py, k.bp.d.Bytes())
		if zx.Sign() == 0 && zy.Sign() == 0 {
			return nil, errors.New("ecka: ECDH produced the point at infinity")
		}
		z := make([]byte, brainpoolByteLen)
		zx.FillBytes(z)
		return kdf.X963SHA256(z, sharedInfo, keyLen)
	default:
		return nil, fmt.Errorf("ecka: unknown curve %d", k.curve)
	}
}

// randScalar returns a uniform private scalar in [1, n-1]. It draws a
// few extra bytes beyond the modulus and reduces, the standard trick
// (FIPS 186-4 Appendix B.4.1) for keeping the modular bias negligible.
func randScalar(n *big.Int) (*big.Int, error) {
	b := make([]byte, (n.BitLen()+7)/8+8)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	d := new(big.Int).SetBytes(b)
	nMinus1 := new(big.Int).Sub(n, big.NewInt(1))
	d.Mod(d, nMinus1)
	d.Add(d, big.NewInt(1)) // shift [0, n-2] → [1, n-1]
	return d, nil
}

// marshalUncompressed encodes (x, y) as 0x04 || X || Y with each
// brainpoolP256r1 coordinate fixed to its byte length.
func marshalUncompressed(x, y *big.Int) []byte {
	out := make([]byte, 1+2*brainpoolByteLen)
	out[0] = 0x04
	x.FillBytes(out[1 : 1+brainpoolByteLen])
	y.FillBytes(out[1+brainpoolByteLen:])
	return out
}

// parseUncompressed parses an uncompressed X9.63 brainpoolP256r1 point.
// It rejects compressed/hybrid encodings, wrong lengths, and the point
// at infinity. The caller must still check the point is on the curve.
func parseUncompressed(data []byte) (x, y *big.Int, err error) {
	if len(data) != 1+2*brainpoolByteLen {
		return nil, nil, fmt.Errorf("ecka: peer pubkey: wrong length %d, want %d", len(data), 1+2*brainpoolByteLen)
	}
	if data[0] != 0x04 {
		return nil, nil, fmt.Errorf("ecka: peer pubkey: unsupported point format 0x%02x (want uncompressed 0x04)", data[0])
	}
	x = new(big.Int).SetBytes(data[1 : 1+brainpoolByteLen])
	y = new(big.Int).SetBytes(data[1+brainpoolByteLen:])
	if x.Sign() == 0 && y.Sign() == 0 {
		return nil, nil, errors.New("ecka: peer pubkey is the point at infinity")
	}
	return x, y, nil
}
