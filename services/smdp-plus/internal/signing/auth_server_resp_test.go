package signing

import (
	"bytes"
	"testing"
)

func TestAuthenticateResponseOk_Roundtrip(t *testing.T) {
	signed := []byte{0x30, 0x03, 0x02, 0x01, 0x05} // SEQUENCE { INTEGER 5 } as a stand-in for EuiccSigned1
	sig := []byte{0xAA, 0xBB, 0xCC}
	leaf := []byte{0x30, 0x02, 0x05, 0x00} // SEQUENCE { NULL } stand-in for a cert
	eum := []byte{0x30, 0x03, 0x02, 0x01, 0x07}

	blob, err := MarshalAuthenticateResponseOk(signed, sig, leaf, eum)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	gotSigned, gotSig, gotLeaf, gotEum, err := UnmarshalAuthenticateResponseOk(blob)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !bytes.Equal(gotSigned, signed) {
		t.Errorf("euiccSigned1 not preserved: %x vs %x", gotSigned, signed)
	}
	if !bytes.Equal(gotSig, sig) {
		t.Errorf("signature not preserved: %x vs %x", gotSig, sig)
	}
	if !bytes.Equal(gotLeaf, leaf) {
		t.Errorf("leaf cert not preserved")
	}
	if !bytes.Equal(gotEum, eum) {
		t.Errorf("eum cert not preserved")
	}
}

func TestMarshalAuthenticateResponseOk_RejectsEmpty(t *testing.T) {
	signed := []byte{0x30, 0x00}
	sig := []byte{0x01}
	cert := []byte{0x30, 0x00}
	cases := []struct {
		name           string
		a1, a2, a3, a4 []byte
	}{
		{"no signed", nil, sig, cert, cert},
		{"no sig", signed, nil, cert, cert},
		{"no leaf", signed, sig, nil, cert},
		{"no eum", signed, sig, cert, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := MarshalAuthenticateResponseOk(c.a1, c.a2, c.a3, c.a4); err == nil {
				t.Errorf("expected error")
			}
		})
	}
}

func TestUnmarshalAuthenticateResponseOk_RejectsTrailingBytes(t *testing.T) {
	signed := []byte{0x30, 0x03, 0x02, 0x01, 0x05}
	sig := []byte{0xAA}
	cert := []byte{0x30, 0x02, 0x05, 0x00}
	blob, _ := MarshalAuthenticateResponseOk(signed, sig, cert, cert)
	blob = append(blob, 0xFF)
	if _, _, _, _, err := UnmarshalAuthenticateResponseOk(blob); err == nil {
		t.Fatal("expected trailing-bytes error")
	}
}

func TestUnmarshalAuthenticateResponseOk_RejectsGarbage(t *testing.T) {
	if _, _, _, _, err := UnmarshalAuthenticateResponseOk([]byte{0x00, 0x01, 0x02}); err == nil {
		t.Fatal("expected error on non-SEQUENCE input")
	}
}
