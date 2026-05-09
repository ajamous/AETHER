package template

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
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
