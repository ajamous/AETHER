package saip

import (
	"bytes"
	"strings"
	"testing"
)

// goodHeader returns a ProfileHeader that passes validate().
func goodHeader() ProfileHeader {
	return ProfileHeader{
		MajorVersion:           SAIPMajorVersion,
		MinorVersion:           SAIPMinorVersion,
		ProfileType:            ProfileTypeGSMA,
		ICCID:                  bytes.Repeat([]byte{0xAA}, 10),
		EUICCMandatoryServices: []string{"contactless", "javacard"},
	}
}

func TestBuild_Roundtrip(t *testing.T) {
	pkg, err := Build(goodHeader())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	der, err := pkg.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// First byte must be SEQUENCE (0x30).
	if der[0] != 0x30 {
		t.Errorf("first byte = 0x%02x, want 0x30", der[0])
	}

	elems, err := Decode(der)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(elems) != 2 {
		t.Fatalf("got %d elements, want 2 (header + end)", len(elems))
	}

	hdr, ok := DecodeHeader(elems[0])
	if !ok {
		t.Fatal("first element should decode as header")
	}
	if hdr.MajorVersion != SAIPMajorVersion {
		t.Errorf("major = %d, want %d", hdr.MajorVersion, SAIPMajorVersion)
	}
	if hdr.ProfileType != ProfileTypeGSMA {
		t.Errorf("profileType = %q", hdr.ProfileType)
	}
	if !bytes.Equal(hdr.ICCID, bytes.Repeat([]byte{0xAA}, 10)) {
		t.Errorf("iccid mismatch")
	}
	if len(hdr.EUICCMandatoryServices) != 2 || hdr.EUICCMandatoryServices[0] != "contactless" {
		t.Errorf("services = %v", hdr.EUICCMandatoryServices)
	}

	if !IsEnd(elems[1]) {
		t.Error("second element should be PEEnd")
	}
	if hdr2, ok := DecodeHeader(elems[1]); ok {
		t.Errorf("PEEnd must NOT decode as header, got %+v", hdr2)
	}
}

func TestBuild_Validation(t *testing.T) {
	cases := []struct {
		name    string
		mut     func(*ProfileHeader)
		wantErr string
	}{
		{
			"major out of range",
			func(h *ProfileHeader) { h.MajorVersion = 0 },
			"MajorVersion",
		},
		{
			"major too large",
			func(h *ProfileHeader) { h.MajorVersion = 100 },
			"MajorVersion",
		},
		{
			"minor out of range",
			func(h *ProfileHeader) { h.MinorVersion = -1 },
			"MinorVersion",
		},
		{
			"empty profileType",
			func(h *ProfileHeader) { h.ProfileType = "" },
			"ProfileType",
		},
		{
			"short ICCID",
			func(h *ProfileHeader) { h.ICCID = []byte{0x01, 0x02} },
			"ICCID",
		},
		{
			"long ICCID",
			func(h *ProfileHeader) { h.ICCID = bytes.Repeat([]byte{0}, 11) },
			"ICCID",
		},
		{
			"empty mandatory services",
			func(h *ProfileHeader) { h.EUICCMandatoryServices = nil },
			"EUICCMandatoryServices",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := goodHeader()
			c.mut(&h)
			_, err := Build(h)
			if err == nil {
				t.Fatalf("expected error containing %q", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q did not mention %q", err.Error(), c.wantErr)
			}
		})
	}
}

func TestDecode_RejectsTrailingBytes(t *testing.T) {
	pkg, _ := Build(goodHeader())
	der, _ := pkg.MarshalDER()
	der = append(der, 0xFF)
	if _, err := Decode(der); err == nil {
		t.Fatal("expected trailing-bytes error")
	}
}

func TestDecode_RejectsNonSequence(t *testing.T) {
	if _, err := Decode([]byte{0x02, 0x01, 0x05}); err == nil {
		t.Fatal("expected outer-SEQUENCE error")
	}
}

func TestDecode_RejectsTruncated(t *testing.T) {
	pkg, _ := Build(goodHeader())
	der, _ := pkg.MarshalDER()
	if _, err := Decode(der[:5]); err == nil {
		t.Fatal("expected truncated-input error")
	}
}

// TestAppendRaw_InsertsBeforeEnd round-trips a hand-rolled spare
// element (a small UTF8String tagged [42]) and confirms it lands
// between the header and the PEEnd marker.
func TestAppendRaw_InsertsBeforeEnd(t *testing.T) {
	pkg, err := Build(goodHeader())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// Synthesize a [42] EXPLICIT-tagged ProfileElement-like blob.
	// Body is "x" as a UTF8String: 0x0C 0x01 'x' = 3 bytes; wrap
	// in CONTEXT-SPECIFIC CONSTRUCTED tag 42 = 0xBA → 0xBA 0x03 ...
	spare := []byte{0xBA, 0x03, 0x0C, 0x01, 'x'}
	if err := pkg.AppendRaw(spare); err != nil {
		t.Fatalf("append: %v", err)
	}
	der, err := pkg.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	elems, err := Decode(der)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(elems) != 3 {
		t.Fatalf("got %d elements, want 3 (header + spare + end)", len(elems))
	}
	if _, ok := DecodeHeader(elems[0]); !ok {
		t.Error("element[0] should be header")
	}
	if !IsEnd(elems[2]) {
		t.Error("element[2] should be PEEnd")
	}
	// element[1] is the spare we inserted.
	if !bytes.Equal(elems[1], spare) {
		t.Errorf("element[1] differs from spare")
	}
}

func TestAppendRaw_RejectsBeforeBuild(t *testing.T) {
	var pkg ProfilePackage
	if err := pkg.AppendRaw([]byte{0xBA, 0x00}); err == nil {
		t.Fatal("expected error on AppendRaw before Build")
	}
}

func TestAppendRaw_RejectsEmpty(t *testing.T) {
	pkg, _ := Build(goodHeader())
	if err := pkg.AppendRaw(nil); err == nil {
		t.Fatal("expected error on empty AppendRaw")
	}
}

// TestMarshalDER_StableAcrossInvocations is a paranoid check that
// two builds of the same logical input produce byte-identical DER.
// SAIP packages get hashed for caching + signed downstream; if
// two builds drift, the cache stops working.
func TestMarshalDER_StableAcrossInvocations(t *testing.T) {
	a, _ := Build(goodHeader())
	b, _ := Build(goodHeader())
	aBytes, _ := a.MarshalDER()
	bBytes, _ := b.MarshalDER()
	if !bytes.Equal(aBytes, bBytes) {
		t.Errorf("DER differs across invocations:\n  a=%x\n  b=%x", aBytes, bBytes)
	}
}

// TestDERLength_Roundtrip exercises the helper directly across the
// short/long-form boundary.
func TestDERLength_Roundtrip(t *testing.T) {
	for _, n := range []int{0, 1, 127, 128, 255, 256, 65535, 65536} {
		enc := derLength(n)
		got, used, err := readDERLength(enc)
		if err != nil {
			t.Errorf("n=%d: read err: %v", n, err)
			continue
		}
		if got != n {
			t.Errorf("n=%d: got %d", n, got)
		}
		if used != len(enc) {
			t.Errorf("n=%d: used %d bytes, encoded %d", n, used, len(enc))
		}
	}
}
