// Package brainpool implements the Brainpool P-256 r1 elliptic curve
// (RFC 5639 §3.4) as a crypto/elliptic Curve.
//
// SGP.22 §2.6.1 mandates support for brainpoolP256r1 alongside NIST
// P-256 in every compliant RSP implementation. The Go standard library
// only ships the NIST/SEC prime curves. crypto/elliptic.CurveParams
// cannot stand in for Brainpool: its generic arithmetic hardcodes the
// curve coefficient a = -3 (true for every NIST prime curve, but NOT
// for the Brainpool family, whose a is an arbitrary field element). A
// CurveParams built from Brainpool's domain parameters reports its own
// base point as off-curve and panics inside ScalarMult. This package
// therefore supplies its own short-Weierstrass arithmetic with a
// general a coefficient, exposed through the standard elliptic.Curve
// interface so that crypto/ecdsa's generic path drives it unchanged.
//
// # Security note
//
// This is a portable math/big implementation. Its field and scalar
// arithmetic are NOT constant-time, so it must not sign with a
// long-lived private key on a host an attacker can co-locate on:
// the variable-time scalar multiplication leaks the nonce and key
// through timing. It is appropriate for:
//
//   - signature verification (no secret scalar), and
//   - lab / ephemeral signing where the key is non-secret or
//     single-use.
//
// For production Brainpool signing, hold the key in an HSM and sign
// through services/hsm-broker, never with this package directly.
package brainpool

import (
	"crypto/elliptic"
	"math/big"
	"sync"
)

// curve is a short-Weierstrass curve y² = x³ + a·x + b over the prime
// field GF(p). Unlike elliptic.CurveParams it does not assume a = -3.
//
// The point at infinity is represented, as in crypto/elliptic, by the
// pair (0, 0); this is safe here because (0, 0) does not satisfy the
// Brainpool curve equation (b is non-zero and not the square root of
// itself), so it can never collide with an affine curve point.
type curve struct {
	params *elliptic.CurveParams
	a      *big.Int
}

var (
	p256r1Once sync.Once
	p256r1     *curve
)

// P256r1 returns the Brainpool P-256 r1 curve (RFC 5639 §3.4).
func P256r1() elliptic.Curve {
	p256r1Once.Do(func() {
		p256r1 = &curve{
			a: mustHex("7D5A0975FC2C3057EEF67530417AFFE7FB8055C126DC5C6CE94A4B44F330B5D9"),
			params: &elliptic.CurveParams{
				Name:    "brainpoolP256r1",
				BitSize: 256,
				P:       mustHex("A9FB57DBA1EEA9BC3E660A909D838D726E3BF623D52620282013481D1F6E5377"),
				N:       mustHex("A9FB57DBA1EEA9BC3E660A909D838D718C397AA3B561A6F7901E0E82974856A7"),
				B:       mustHex("26DC5C6CE94A4B44F330B5D9BBD77CBF958416295CF7E1CE6BCCDC18FF8C07B6"),
				Gx:      mustHex("8BD2AEB9CB7E57CB2C4B482FFC81B7AFB9DE27E1E3BD23C23A4453BD9ACE3262"),
				Gy:      mustHex("547EF835C3DAC4FD97F8461A14611DC9C27745132DED8E545C1D54C72F046997"),
			},
		}
	})
	return p256r1
}

func mustHex(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 16)
	if !ok {
		panic("brainpool: invalid hex constant " + s)
	}
	return n
}

// Params returns the curve's domain parameters. The returned
// CurveParams carries the correct P, N, B, Gx, Gy and BitSize, but its
// own arithmetic methods (which assume a = -3) must not be used; callers
// go through this curve's methods instead.
func (c *curve) Params() *elliptic.CurveParams { return c.params }

func (c *curve) isInfinity(x, y *big.Int) bool {
	return x.Sign() == 0 && y.Sign() == 0
}

// IsOnCurve reports whether (x, y) satisfies y² = x³ + a·x + b (mod p).
func (c *curve) IsOnCurve(x, y *big.Int) bool {
	p := c.params.P
	if x.Sign() < 0 || y.Sign() < 0 || x.Cmp(p) >= 0 || y.Cmp(p) >= 0 {
		return false
	}
	// y² mod p
	left := new(big.Int).Mul(y, y)
	left.Mod(left, p)
	// x³ + a·x + b mod p
	right := new(big.Int).Mul(x, x)
	right.Mul(right, x)            // x³
	ax := new(big.Int).Mul(c.a, x) // a·x
	right.Add(right, ax)
	right.Add(right, c.params.B)
	right.Mod(right, p)
	return left.Cmp(right) == 0
}

// Add returns the sum of (x1, y1) and (x2, y2) using affine
// short-Weierstrass addition.
func (c *curve) Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	if c.isInfinity(x1, y1) {
		return new(big.Int).Set(x2), new(big.Int).Set(y2)
	}
	if c.isInfinity(x2, y2) {
		return new(big.Int).Set(x1), new(big.Int).Set(y1)
	}
	p := c.params.P
	if x1.Cmp(x2) == 0 {
		// Same x: either doubling, or P + (-P) = ∞.
		if y1.Cmp(y2) == 0 && y1.Sign() != 0 {
			return c.Double(x1, y1)
		}
		return new(big.Int), new(big.Int) // infinity
	}
	// λ = (y2 - y1) / (x2 - x1) mod p
	num := new(big.Int).Sub(y2, y1)
	den := new(big.Int).Sub(x2, x1)
	den.ModInverse(den.Mod(den, p), p)
	lambda := num.Mul(num, den)
	lambda.Mod(lambda, p)
	return c.fromLambda(lambda, x1, y1, x2)
}

// Double returns 2·(x1, y1).
func (c *curve) Double(x1, y1 *big.Int) (*big.Int, *big.Int) {
	if c.isInfinity(x1, y1) || y1.Sign() == 0 {
		return new(big.Int), new(big.Int) // infinity
	}
	p := c.params.P
	// λ = (3·x1² + a) / (2·y1) mod p
	num := new(big.Int).Mul(x1, x1)
	num.Mul(num, big.NewInt(3))
	num.Add(num, c.a)
	den := new(big.Int).Lsh(y1, 1) // 2·y1
	den.ModInverse(den.Mod(den, p), p)
	lambda := num.Mul(num, den)
	lambda.Mod(lambda, p)
	return c.fromLambda(lambda, x1, y1, x1)
}

// fromLambda computes the resulting point from a slope λ for both the
// addition (x2 ≠ x1) and doubling (x2 = x1) cases:
//
//	x3 = λ² - x1 - x2 ; y3 = λ·(x1 - x3) - y1   (mod p)
func (c *curve) fromLambda(lambda, x1, y1, x2 *big.Int) (*big.Int, *big.Int) {
	p := c.params.P
	x3 := new(big.Int).Mul(lambda, lambda)
	x3.Sub(x3, x1)
	x3.Sub(x3, x2)
	x3.Mod(x3, p)
	y3 := new(big.Int).Sub(x1, x3)
	y3.Mul(y3, lambda)
	y3.Sub(y3, y1)
	y3.Mod(y3, p)
	return x3, y3
}

// ScalarMult returns k·(Bx, By) via left-to-right double-and-add.
//
// Not constant-time: the conditional add is data-dependent. See the
// package security note.
func (c *curve) ScalarMult(Bx, By *big.Int, k []byte) (*big.Int, *big.Int) {
	x, y := new(big.Int), new(big.Int) // infinity
	for _, b := range k {
		for bit := 0; bit < 8; bit++ {
			x, y = c.Double(x, y)
			if b&0x80 != 0 {
				x, y = c.Add(x, y, Bx, By)
			}
			b <<= 1
		}
	}
	return x, y
}

// ScalarBaseMult returns k·G where G is the curve's base point.
func (c *curve) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	return c.ScalarMult(c.params.Gx, c.params.Gy, k)
}
