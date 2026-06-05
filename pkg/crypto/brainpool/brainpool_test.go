package brainpool

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"math/big"
	"testing"
)

func hexInt(t *testing.T, s string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(s, 16)
	if !ok {
		t.Fatalf("bad hex %q", s)
	}
	return n
}

// TestBasePointOnCurve guards the domain parameters: if a, b, p, or G
// were mistyped, the published base point would not satisfy the curve
// equation. This is the check elliptic.CurveParams fails for Brainpool.
func TestBasePointOnCurve(t *testing.T) {
	p := Params()
	if !IsOnCurve(p.Gx, p.Gy) {
		t.Fatal("base point G is not on the curve — domain parameters are wrong")
	}
}

// TestOrderAnnihilatesBasePoint checks N·G = ∞, which exercises the full
// scalar-multiplication path against the published group order.
func TestOrderAnnihilatesBasePoint(t *testing.T) {
	x, y := ScalarBaseMult(Params().N.Bytes())
	if x.Sign() != 0 || y.Sign() != 0 {
		t.Fatalf("N·G = (%s, %s), want point at infinity", x, y)
	}
}

// TestRFC7027ECDH validates point arithmetic against the official
// Brainpool P-256 r1 ECDH test vector from RFC 7027 §A.1. Both parties
// must derive the same shared X coordinate, and it must equal the
// published value. A single arithmetic bug (slope, reduction, sign)
// would break this.
func TestRFC7027ECDH(t *testing.T) {
	dA := hexInt(t, "81DB1EE100150FF2EA338D708271BE38300CB54241D79950F77B063039804F1D")
	xqA := hexInt(t, "44106E913F92BC02A1705D9953A8414DB95E1AAA49E81D9E85F929A8E3100BE5")
	yqA := hexInt(t, "8AB4846F11CACCB73CE49CBDD120F5A900A69FD32C272223F789EF10EB089BDC")

	dB := hexInt(t, "55E40BC41E37E3E2AD25C3C6654511FFA8474A91A0032087593852D3E7D76BD3")
	xqB := hexInt(t, "8D2D688C6CF93E1160AD04CC4429117DC2C41825E1E9FCA0ADDD34E6F1B39F7B")
	yqB := hexInt(t, "990C57520812BE512641E47034832106BC7D3E8DD0E4C7F1136D7006547CEC6A")

	wantZx := hexInt(t, "89AFC39D41D3B327814B80940B042590F96556EC91E6AE7939BCE31F3A18BF2B")

	// Public keys must be reproduced from the private scalars.
	if gotX, gotY := ScalarBaseMult(dA.Bytes()); gotX.Cmp(xqA) != 0 || gotY.Cmp(yqA) != 0 {
		t.Fatalf("QA mismatch:\n got (%x, %x)\nwant (%x, %x)", gotX, gotY, xqA, yqA)
	}
	if gotX, gotY := ScalarBaseMult(dB.Bytes()); gotX.Cmp(xqB) != 0 || gotY.Cmp(yqB) != 0 {
		t.Fatalf("QB mismatch:\n got (%x, %x)\nwant (%x, %x)", gotX, gotY, xqB, yqB)
	}

	// Both ECDH directions must agree with the published shared X.
	zx1, _ := ScalarMult(xqB, yqB, dA.Bytes())
	zx2, _ := ScalarMult(xqA, yqA, dB.Bytes())
	if zx1.Cmp(zx2) != 0 {
		t.Fatalf("ECDH not symmetric: %x != %x", zx1, zx2)
	}
	if zx1.Cmp(wantZx) != 0 {
		t.Fatalf("shared secret X mismatch:\n got %x\nwant %x", zx1, wantZx)
	}
}

// TestECDSARoundTrip confirms crypto/ecdsa drives the custom curve end
// to end: key generation, signing, and verification all succeed, and a
// tampered digest is rejected.
func TestECDSARoundTrip(t *testing.T) {
	priv, err := ecdsa.GenerateKey(P256r1(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	digest := sha256.Sum256([]byte("aether brainpool round trip"))
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !ecdsa.Verify(&priv.PublicKey, digest[:], r, s) {
		t.Fatal("valid signature failed to verify")
	}
	bad := sha256.Sum256([]byte("different message"))
	if ecdsa.Verify(&priv.PublicKey, bad[:], r, s) {
		t.Fatal("signature verified against the wrong digest")
	}
}

// TestAddCommutativeAndInverse exercises the addition special cases:
// commutativity, identity, and P + (-P) = ∞.
func TestAddCommutativeAndInverse(t *testing.T) {
	p := Params()
	// 2G and 3G as sample points.
	x2, y2 := Double(p.Gx, p.Gy)
	x3, y3 := Add(x2, y2, p.Gx, p.Gy)

	// Commutativity: 2G + 3G == 3G + 2G.
	ax, ay := Add(x2, y2, x3, y3)
	bx, by := Add(x3, y3, x2, y2)
	if ax.Cmp(bx) != 0 || ay.Cmp(by) != 0 {
		t.Fatal("addition is not commutative")
	}

	// Identity: G + ∞ == G.
	ix, iy := Add(p.Gx, p.Gy, new(big.Int), new(big.Int))
	if ix.Cmp(p.Gx) != 0 || iy.Cmp(p.Gy) != 0 {
		t.Fatal("G + ∞ != G")
	}

	// Inverse: G + (-G) == ∞, where -G = (Gx, p - Gy).
	negGy := new(big.Int).Sub(p.P, p.Gy)
	zx, zy := Add(p.Gx, p.Gy, p.Gx, negGy)
	if zx.Sign() != 0 || zy.Sign() != 0 {
		t.Fatalf("G + (-G) = (%s, %s), want ∞", zx, zy)
	}
}
