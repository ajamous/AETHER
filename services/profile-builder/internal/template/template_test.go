package template

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ajamous/aether/pkg/saip"
)

const sampleYAML = `name: lab-mvno
description: Test profile for the local lab
version: "1.0"
network:
  mcc: "001"
  mnc: "01"
  plmn_name: Aether Lab Network
  hplmn_act: GSM
naa:
  apps:
    - USIM
ota:
  kic: AAECAwQFBgcICQoLDA0ODw==
  kid: EBESExQVFhcYGRobHB0eHw==
  spi: AAA=
`

func TestParseAndValidate(t *testing.T) {
	p, err := Parse([]byte(sampleYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if p.Name != "lab-mvno" {
		t.Fatalf("name = %q", p.Name)
	}
}

func TestValidate_RejectsBadMCC(t *testing.T) {
	p, _ := Parse([]byte(sampleYAML))
	p.Network.MCC = "12"
	if err := p.Validate(); err == nil {
		t.Fatal("expected error on short MCC")
	}
}

func TestValidate_RejectsUnknownNAA(t *testing.T) {
	p, _ := Parse([]byte(sampleYAML))
	p.NAA.Apps = []string{"FAKEAPP"}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error on unknown NAA app")
	}
}

func TestBuildUPP_HappyPath(t *testing.T) {
	p, _ := Parse([]byte(sampleYAML))
	sub := &SubscriberData{
		IMSI:   "001010000000001",
		ICCID:  "8900000000000000001",
		MSISDN: "10000001",
		Ki:     bytes.Repeat([]byte{0xAA}, 16),
		OPc:    bytes.Repeat([]byte{0xBB}, 16),
	}
	upp, err := BuildUPP(p, sub)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if upp.Subscriber.IMSI != sub.IMSI {
		t.Fatalf("subscriber IMSI not propagated")
	}
}

func TestBuildUPP_RejectsBadIMSI(t *testing.T) {
	p, _ := Parse([]byte(sampleYAML))
	sub := &SubscriberData{
		IMSI: "abc",
		ICCID: "8900000000000000001",
		Ki: bytes.Repeat([]byte{0}, 16),
		OPc: bytes.Repeat([]byte{0}, 16),
	}
	if _, err := BuildUPP(p, sub); err == nil {
		t.Fatal("expected error on bad IMSI")
	}
}

func TestLoader_List(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.yaml", "b.yml", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(sampleYAML), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	l := NewLoader(dir)
	names, err := l.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 templates, got %d: %v", len(names), names)
	}
}

// TestBuildUPP_EmitsValidSAIP confirms BuildUPP now produces a
// real DER-encoded SAIP ProfilePackage. The bytes round-trip
// through pkg/saip and the header decodes with the expected
// fields.
func TestBuildUPP_EmitsValidSAIP(t *testing.T) {
	p, _ := Parse([]byte(sampleYAML))
	sub := &SubscriberData{
		IMSI:   "001019912345678",
		ICCID:  "8900000000000000001",
		MSISDN: "1234567",
		Ki:     bytes.Repeat([]byte{0xAA}, 16),
		OPc:    bytes.Repeat([]byte{0xBB}, 16),
	}
	upp, err := BuildUPP(p, sub)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(upp.SAIP) == 0 {
		t.Fatal("upp.SAIP is empty — SAIP encoding did not run")
	}
	if upp.SAIP[0] != 0x30 {
		t.Errorf("SAIP first byte = 0x%02x, want 0x30 (SEQUENCE)", upp.SAIP[0])
	}
	elems, err := saip.Decode(upp.SAIP)
	if err != nil {
		t.Fatalf("saip.Decode: %v", err)
	}
	if len(elems) != 2 {
		t.Fatalf("got %d SAIP elements, want 2 (header + end)", len(elems))
	}
	hdr, ok := saip.DecodeHeader(elems[0])
	if !ok {
		t.Fatal("element[0] should decode as SAIP header")
	}
	if hdr.MajorVersion != saip.SAIPMajorVersion {
		t.Errorf("major = %d, want %d", hdr.MajorVersion, saip.SAIPMajorVersion)
	}
	if hdr.ProfileType != p.Name {
		t.Errorf("profileType = %q, want template name %q", hdr.ProfileType, p.Name)
	}
	if len(hdr.ICCID) != 10 {
		t.Errorf("ICCID = %d bytes, want 10", len(hdr.ICCID))
	}
	if !saip.IsEnd(elems[1]) {
		t.Error("element[1] should be PEEnd")
	}
}

// TestEncodeICCIDNibbleSwapped covers the BCD nibble-swap shape
// SGP.22 §B.1 requires.
func TestEncodeICCIDNibbleSwapped(t *testing.T) {
	cases := []struct {
		in   string
		want []byte
	}{
		// 20 digits: each pair (d1,d2) → 0x[d2][d1].
		{"89012345678901234567", []byte{0x98, 0x10, 0x32, 0x54, 0x76, 0x98, 0x10, 0x32, 0x54, 0x76}},
		// 19 digits: pad with F nibble.
		{"8901234567890123456", []byte{0x98, 0x10, 0x32, 0x54, 0x76, 0x98, 0x10, 0x32, 0x54, 0xF6}},
	}
	for _, c := range cases {
		got, err := encodeICCIDNibbleSwapped(c.in)
		if err != nil {
			t.Errorf("encode(%q): %v", c.in, err)
			continue
		}
		if !bytes.Equal(got, c.want) {
			t.Errorf("encode(%q) = %x, want %x", c.in, got, c.want)
		}
	}
}

func TestEncodeICCIDNibbleSwapped_Rejects(t *testing.T) {
	for _, in := range []string{"", "12345", "8901234567890123456789012345"} {
		if _, err := encodeICCIDNibbleSwapped(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}
