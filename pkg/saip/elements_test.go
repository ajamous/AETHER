package saip

import (
	"bytes"
	"strings"
	"testing"
)

func goodUSIM() PEUSIM {
	return PEUSIM{IMSI: "001010000000001", MCC: "001", MNC: "01"}
}

func goodAKA() PEAKAParameter {
	return PEAKAParameter{
		AlgorithmID: AKAAlgorithmMilenage,
		Ki:          bytes.Repeat([]byte{0xAA}, MilenageKeyLen),
		OPc:         bytes.Repeat([]byte{0xBB}, MilenageKeyLen),
	}
}

func TestBuildUSIM_Roundtrip(t *testing.T) {
	der, err := BuildUSIM(goodUSIM())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	tag, n, err := peekTLV(der)
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if tag != tagUSIM {
		t.Errorf("tag = %d, want %d", tag, tagUSIM)
	}
	if n != len(der) {
		t.Errorf("peek span %d != len %d", n, len(der))
	}
	got, ok := DecodeUSIM(der)
	if !ok {
		t.Fatal("DecodeUSIM returned !ok")
	}
	if got != goodUSIM() {
		t.Errorf("roundtrip mismatch: %+v != %+v", got, goodUSIM())
	}
}

func TestBuildAKAParameter_Roundtrip(t *testing.T) {
	der, err := BuildAKAParameter(goodAKA())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	tag, n, err := peekTLV(der)
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if tag != tagAKAParameter {
		t.Errorf("tag = %d, want %d", tag, tagAKAParameter)
	}
	if n != len(der) {
		t.Errorf("peek span %d != len %d", n, len(der))
	}
	got, ok := DecodeAKAParameter(der)
	if !ok {
		t.Fatal("DecodeAKAParameter returned !ok")
	}
	if got.AlgorithmID != AKAAlgorithmMilenage {
		t.Errorf("algo = %d", got.AlgorithmID)
	}
	if !bytes.Equal(got.Ki, goodAKA().Ki) {
		t.Errorf("Ki mismatch")
	}
	if !bytes.Equal(got.OPc, goodAKA().OPc) {
		t.Errorf("OPc mismatch")
	}
}

// TestElements_DistinctTagsFromHeaderAndEnd guards the one property
// the encoder relies on: the credential elements must not collide
// with the header [0] or end [99] CHOICE tags, otherwise Decode
// dispatch (DecodeHeader / IsEnd) would misclassify them.
func TestElements_DistinctTagsFromHeaderAndEnd(t *testing.T) {
	usimDER, _ := BuildUSIM(goodUSIM())
	akaDER, _ := BuildAKAParameter(goodAKA())
	for _, der := range [][]byte{usimDER, akaDER} {
		if _, ok := DecodeHeader(der); ok {
			t.Error("credential element wrongly decoded as header")
		}
		if IsEnd(der) {
			t.Error("credential element wrongly classified as PEEnd")
		}
	}
}

// TestElements_InProfilePackage builds a full package with the
// header, both credential elements, and the terminator, then decodes
// it and confirms each element lands in the right slot and value.
func TestElements_InProfilePackage(t *testing.T) {
	pkg, err := Build(goodHeader())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	usimDER, err := BuildUSIM(goodUSIM())
	if err != nil {
		t.Fatalf("usim: %v", err)
	}
	akaDER, err := BuildAKAParameter(goodAKA())
	if err != nil {
		t.Fatalf("aka: %v", err)
	}
	if err := pkg.AppendRaw(usimDER); err != nil {
		t.Fatalf("append usim: %v", err)
	}
	if err := pkg.AppendRaw(akaDER); err != nil {
		t.Fatalf("append aka: %v", err)
	}

	der, err := pkg.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	elems, err := Decode(der)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(elems) != 4 {
		t.Fatalf("got %d elements, want 4 (header + usim + aka + end)", len(elems))
	}
	if _, ok := DecodeHeader(elems[0]); !ok {
		t.Error("element[0] should be header")
	}
	if u, ok := DecodeUSIM(elems[1]); !ok || u.IMSI != goodUSIM().IMSI {
		t.Errorf("element[1] should be PE-USIM with IMSI, got ok=%v u=%+v", ok, u)
	}
	if a, ok := DecodeAKAParameter(elems[2]); !ok || !bytes.Equal(a.Ki, goodAKA().Ki) {
		t.Errorf("element[2] should be PE-AKAParameter with Ki, got ok=%v", ok)
	}
	if !IsEnd(elems[3]) {
		t.Error("element[3] should be PEEnd")
	}
}

func TestBuildUSIM_Validation(t *testing.T) {
	cases := []struct {
		name    string
		mut     func(*PEUSIM)
		wantErr string
	}{
		{"empty IMSI", func(u *PEUSIM) { u.IMSI = "" }, "IMSI"},
		{"non-digit IMSI", func(u *PEUSIM) { u.IMSI = "00101000000000X" }, "IMSI"},
		{"long IMSI", func(u *PEUSIM) { u.IMSI = "0010100000000010" }, "IMSI"},
		{"short MCC", func(u *PEUSIM) { u.MCC = "01" }, "MCC"},
		{"long MNC", func(u *PEUSIM) { u.MNC = "0011" }, "MNC"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := goodUSIM()
			c.mut(&u)
			_, err := BuildUSIM(u)
			if err == nil {
				t.Fatalf("expected error containing %q", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q did not mention %q", err.Error(), c.wantErr)
			}
		})
	}
}

func TestBuildAKAParameter_Validation(t *testing.T) {
	cases := []struct {
		name    string
		mut     func(*PEAKAParameter)
		wantErr string
	}{
		{"unknown algo", func(a *PEAKAParameter) { a.AlgorithmID = 99 }, "AlgorithmID"},
		{"short Ki", func(a *PEAKAParameter) { a.Ki = []byte{0x01} }, "Ki"},
		{"short OPc", func(a *PEAKAParameter) { a.OPc = bytes.Repeat([]byte{0}, 8) }, "OPc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := goodAKA()
			c.mut(&a)
			_, err := BuildAKAParameter(a)
			if err == nil {
				t.Fatalf("expected error containing %q", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q did not mention %q", err.Error(), c.wantErr)
			}
		})
	}
}

// TestElements_StableDER guards the caching/signing invariant the
// package relies on elsewhere: identical inputs marshal to identical
// bytes.
func TestElements_StableDER(t *testing.T) {
	a1, _ := BuildAKAParameter(goodAKA())
	a2, _ := BuildAKAParameter(goodAKA())
	if !bytes.Equal(a1, a2) {
		t.Error("PE-AKAParameter DER not stable across invocations")
	}
	u1, _ := BuildUSIM(goodUSIM())
	u2, _ := BuildUSIM(goodUSIM())
	if !bytes.Equal(u1, u2) {
		t.Error("PE-USIM DER not stable across invocations")
	}
}
