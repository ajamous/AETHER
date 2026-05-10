package sgp22

import (
	"bytes"
	"testing"
)

func TestPEHeader_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   PEHeader
	}{
		{
			name: "minimal",
			in: PEHeader{
				MandatoryFlag: true,
				IccidPresent:  false,
				Identifier:    []byte{0x01, 0x02, 0x03},
			},
		},
		{
			name: "all flags set, longer identifier",
			in: PEHeader{
				MandatoryFlag: true,
				IccidPresent:  true,
				Identifier:    bytes.Repeat([]byte{0xAB}, 16),
			},
		},
		{
			name: "empty identifier",
			in: PEHeader{
				MandatoryFlag: false,
				IccidPresent:  false,
				Identifier:    []byte{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := tc.in.Marshal()
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			out, err := UnmarshalPEHeader(encoded)
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if out.MandatoryFlag != tc.in.MandatoryFlag {
				t.Errorf("MandatoryFlag: got %v, want %v", out.MandatoryFlag, tc.in.MandatoryFlag)
			}
			if out.IccidPresent != tc.in.IccidPresent {
				t.Errorf("IccidPresent: got %v, want %v", out.IccidPresent, tc.in.IccidPresent)
			}
			if !bytes.Equal(out.Identifier, tc.in.Identifier) {
				t.Errorf("Identifier: got %x, want %x", out.Identifier, tc.in.Identifier)
			}
		})
	}
}

func TestPEHeader_TrailingBytesRejected(t *testing.T) {
	in := PEHeader{
		MandatoryFlag: true,
		IccidPresent:  true,
		Identifier:    []byte{0x10},
	}
	encoded, err := in.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	garbage := make([]byte, 0, len(encoded)+2)
	garbage = append(garbage, encoded...)
	garbage = append(garbage, 0xFF, 0xFF)
	if _, err := UnmarshalPEHeader(garbage); err == nil {
		t.Fatal("expected error on trailing bytes, got nil")
	}
}

func TestUnmarshalPEHeader_GarbageRejected(t *testing.T) {
	if _, err := UnmarshalPEHeader([]byte{0x00, 0x01, 0x02}); err == nil {
		t.Fatal("expected error on garbage input, got nil")
	}
}
