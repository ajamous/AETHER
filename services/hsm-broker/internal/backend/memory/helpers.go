package memory

import (
	"crypto/ecdsa"
	"crypto/elliptic"
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
	default:
		return 0, fmt.Errorf("%w: %q", broker.ErrUnsupportedCurve, c)
	}
}

// ecdsaPubBytes returns the uncompressed X9.63 point encoding of pub.
func ecdsaPubBytes(priv *ecdsa.PrivateKey) []byte {
	return elliptic.Marshal(priv.Curve, priv.PublicKey.X, priv.PublicKey.Y)
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
