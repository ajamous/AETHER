package memory

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/asn1"
	"fmt"
	"math/big"

	hsmv1 "github.com/ajamous/aether/services/hsm-broker/api/v1"
	"github.com/ajamous/aether/services/hsm-broker/internal/broker"

	cryptoecdsa "github.com/ajamous/aether/pkg/crypto/ecdsa"
	cryptoecka "github.com/ajamous/aether/pkg/crypto/ecka"
)

func mapECDSACurve(c hsmv1.Curve) (cryptoecdsa.Curve, error) {
	switch c {
	case hsmv1.CurveP256:
		return cryptoecdsa.CurveP256, nil
	case hsmv1.CurveBrainpoolP256r1:
		return cryptoecdsa.CurveBrainpoolP256r1, nil
	case hsmv1.CurveUnspecified:
		return 0, fmt.Errorf("%w: curve not specified", broker.ErrUnsupportedCurve)
	default:
		return 0, fmt.Errorf("%w: %q", broker.ErrUnsupportedCurve, c)
	}
}

func mapECKACurve(c hsmv1.Curve) (cryptoecka.Curve, error) {
	switch c {
	case hsmv1.CurveP256:
		return cryptoecka.CurveP256, nil
	case hsmv1.CurveBrainpoolP256r1:
		return cryptoecka.CurveBrainpoolP256r1, nil
	case hsmv1.CurveUnspecified:
		return 0, fmt.Errorf("%w: curve not specified", broker.ErrUnsupportedCurve)
	default:
		return 0, fmt.Errorf("%w: %q", broker.ErrUnsupportedCurve, c)
	}
}

// ecdsaPubBytes returns the uncompressed X9.63 point encoding of the
// public key (SEC1 §2.3.3: `0x04 || X || Y`, with X and Y left-padded
// to the curve's byte length).
func ecdsaPubBytes(priv *ecdsa.PrivateKey) []byte {
	// Standard curves: the non-deprecated PublicKey.Bytes() returns the
	// uncompressed SEC1 point directly.
	if b, err := priv.PublicKey.Bytes(); err == nil {
		return b
	}
	// Custom curves (Brainpool P-256 r1) are rejected by
	// PublicKey.Bytes, and crypto/ecdh — the deprecation's suggested
	// replacement — supports NIST curves only. Reading the affine
	// coordinates is the only encoding path that exists for them.
	byteLen := (priv.Curve.Params().BitSize + 7) / 8
	out := make([]byte, 1+2*byteLen)
	out[0] = 0x04
	priv.PublicKey.X.FillBytes(out[1 : 1+byteLen]) //nolint:staticcheck // SA1019: no non-deprecated encoding exists for a custom-curve ecdsa.PublicKey
	priv.PublicKey.Y.FillBytes(out[1+byteLen:])    //nolint:staticcheck // SA1019: see above
	return out
}

type asn1Sig struct {
	R, S *big.Int
}

// signDigest signs a pre-computed digest with priv and returns the
// DER-encoded SEQUENCE { r, s } shape SGP.22 §H.5 expects.
func signDigest(priv *ecdsa.PrivateKey, digest []byte) ([]byte, error) {
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest)
	if err != nil {
		return nil, fmt.Errorf("memory: ecdsa sign: %w", err)
	}
	return asn1.Marshal(asn1Sig{R: r, S: s})
}
