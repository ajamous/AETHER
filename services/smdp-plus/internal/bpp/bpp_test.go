package bpp

import (
	"bytes"
	"strings"
	"testing"
)

func goodISCR() InitialiseSecureChannelRequest {
	return InitialiseSecureChannelRequest{
		RemoteOpId:    RemoteOpIdInstallBoundProfilePackage,
		TransactionID: bytes.Repeat([]byte{0xAA}, 8),
		ControlRefTemplate: ControlRefTemplate{
			KeyUsageQualifier: KeyUsageQualifierEncryptAndIntegrity,
			KeyType:           KeyTypeAESGCM,
			KeyLength:         KeyLengthAES128,
		},
		SMDPOtpk: append([]byte{0x04}, bytes.Repeat([]byte{0xBB}, 64)...),
		SMDPSign: bytes.Repeat([]byte{0xCC}, 70),
	}
}

func TestISCR_RoundTrip(t *testing.T) {
	in := goodISCR()
	der, err := in.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := UnmarshalInitialiseSecureChannelRequest(der)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.RemoteOpId != in.RemoteOpId {
		t.Errorf("RemoteOpId mismatch")
	}
	if !bytes.Equal(out.TransactionID, in.TransactionID) {
		t.Errorf("TransactionID mismatch")
	}
	if !bytes.Equal(out.SMDPOtpk, in.SMDPOtpk) {
		t.Errorf("SMDPOtpk mismatch")
	}
	if !bytes.Equal(out.SMDPSign, in.SMDPSign) {
		t.Errorf("SMDPSign mismatch")
	}
	if !bytes.Equal(out.ControlRefTemplate.KeyUsageQualifier, in.ControlRefTemplate.KeyUsageQualifier) {
		t.Errorf("KeyUsageQualifier mismatch")
	}
}

func TestISCR_Validation(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*InitialiseSecureChannelRequest)
		want string
	}{
		{
			"empty tid",
			func(r *InitialiseSecureChannelRequest) { r.TransactionID = nil },
			"TransactionID",
		},
		{
			"oversized tid",
			func(r *InitialiseSecureChannelRequest) { r.TransactionID = bytes.Repeat([]byte{0}, 17) },
			"TransactionID",
		},
		{
			"otpk wrong length",
			func(r *InitialiseSecureChannelRequest) { r.SMDPOtpk = []byte{0x04, 0x01} },
			"SMDPOtpk",
		},
		{
			"empty signature",
			func(r *InitialiseSecureChannelRequest) { r.SMDPSign = nil },
			"SMDPSign",
		},
		{
			"remoteOpId out of range",
			func(r *InitialiseSecureChannelRequest) { r.RemoteOpId = -1 },
			"RemoteOpId",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := goodISCR()
			c.mut(&r)
			_, err := r.MarshalDER()
			if err == nil {
				t.Fatalf("expected error containing %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q did not mention %q", err.Error(), c.want)
			}
		})
	}
}

func TestSignedInputBytes_Concatenation(t *testing.T) {
	tid := []byte{0x01, 0x02, 0x03}
	smdp := []byte{0x10, 0x11}
	euicc := []byte{0x20, 0x21, 0x22, 0x23}
	got := SignedInputBytes(tid, smdp, euicc)
	want := append(append(append([]byte{}, tid...), smdp...), euicc...)
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestAssembleBoundProfilePackage_OuterTag(t *testing.T) {
	iscr := goodISCR()
	segments := [][]byte{
		bytes.Repeat([]byte{0x11}, 32),
		bytes.Repeat([]byte{0x22}, 32),
	}
	der, err := AssembleBoundProfilePackage(iscr, segments)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	// First byte must be the [APPLICATION 54] constructed tag.
	// Tag 54 ≥ 31 so high-tag-number form is used:
	//   0x7F = APPLICATION (01) | constructed (1) | high-tag (11111)
	// followed by the tag number's VLQ encoding (54 = 0x36, single byte).
	if len(der) < 2 {
		t.Fatalf("der too short: %d bytes", len(der))
	}
	if der[0] != 0x7F {
		t.Errorf("first byte = 0x%02x, want 0x7F (APPLICATION constructed high-tag)", der[0])
	}
	if der[1] != 0x36 {
		t.Errorf("tag VLQ = 0x%02x, want 0x36 (= 54)", der[1])
	}
}

func TestAssembleBoundProfilePackage_NoSegmentsRejected(t *testing.T) {
	iscr := goodISCR()
	if _, err := AssembleBoundProfilePackage(iscr, nil); err == nil {
		t.Fatal("expected error on no segments")
	}
}

func TestAssembleBoundProfilePackage_InvalidISCRRejected(t *testing.T) {
	iscr := goodISCR()
	iscr.TransactionID = nil // would fail validate()
	if _, err := AssembleBoundProfilePackage(iscr, [][]byte{{0x01}}); err == nil {
		t.Fatal("expected error on invalid ISCR")
	}
}

// TestAssembleBoundProfilePackage_StableAcrossInvocations is a
// paranoid check that two assembles of the same logical input
// produce byte-identical DER. BPPs get hashed for caching by some
// adopters; if two builds drift, the cache stops working.
func TestAssembleBoundProfilePackage_StableAcrossInvocations(t *testing.T) {
	iscr := goodISCR()
	segs := [][]byte{bytes.Repeat([]byte{0x42}, 64)}
	a, _ := AssembleBoundProfilePackage(iscr, segs)
	b, _ := AssembleBoundProfilePackage(iscr, segs)
	if !bytes.Equal(a, b) {
		t.Errorf("DER differs across invocations: a=%x b=%x", a, b)
	}
}

// TestDisassembleBoundProfilePackage_RoundTrip confirms a BPP built
// by AssembleBoundProfilePackage parses back into the same ISCR
// fields and the same ordered segment bodies.
func TestDisassembleBoundProfilePackage_RoundTrip(t *testing.T) {
	iscr := goodISCR()
	segments := [][]byte{
		bytes.Repeat([]byte{0x11}, 48),
		bytes.Repeat([]byte{0x22}, 16),
		bytes.Repeat([]byte{0x33}, 80),
	}
	der, err := AssembleBoundProfilePackage(iscr, segments)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	gotISCR, gotSegs, err := DisassembleBoundProfilePackage(der)
	if err != nil {
		t.Fatalf("disassemble: %v", err)
	}
	if gotISCR.RemoteOpId != iscr.RemoteOpId {
		t.Errorf("RemoteOpId = %d, want %d", gotISCR.RemoteOpId, iscr.RemoteOpId)
	}
	if !bytes.Equal(gotISCR.TransactionID, iscr.TransactionID) {
		t.Errorf("TransactionID mismatch")
	}
	if !bytes.Equal(gotISCR.SMDPOtpk, iscr.SMDPOtpk) {
		t.Errorf("SMDPOtpk mismatch")
	}
	if !bytes.Equal(gotISCR.SMDPSign, iscr.SMDPSign) {
		t.Errorf("SMDPSign mismatch")
	}
	if len(gotSegs) != len(segments) {
		t.Fatalf("got %d segments, want %d", len(gotSegs), len(segments))
	}
	for i := range segments {
		if !bytes.Equal(gotSegs[i], segments[i]) {
			t.Errorf("segment %d mismatch", i)
		}
	}
}

func TestDisassembleBoundProfilePackage_RejectsGarbage(t *testing.T) {
	for _, b := range [][]byte{nil, {0x30, 0x01, 0x00}, {0x7F}} {
		if _, _, err := DisassembleBoundProfilePackage(b); err == nil {
			t.Errorf("expected error for %x", b)
		}
	}
}

// TestSharedInfo_Deterministic confirms SharedInfo is stable for a
// given transaction id and binds the id (different ids → different
// bytes). The SM-DP+ and eUICC must derive identical shared-info or
// every segment fails GCM authentication.
func TestSharedInfo_Deterministic(t *testing.T) {
	a := SharedInfo("deadbeef")
	b := SharedInfo("deadbeef")
	if !bytes.Equal(a, b) {
		t.Error("SharedInfo not deterministic for same transaction id")
	}
	if bytes.Equal(a, SharedInfo("cafebabe")) {
		t.Error("SharedInfo does not bind the transaction id")
	}
	// Must carry the SCP03t key parameters up front.
	if !bytes.HasPrefix(a, KeyTypeAESGCM) {
		t.Error("SharedInfo does not lead with keyType")
	}
}

func TestDERLength_RoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 127, 128, 255, 256, 65535, 65536} {
		enc := derLength(n)
		// Manually decode: short form if first byte < 0x80; long
		// form otherwise.
		var got int
		if enc[0] < 0x80 {
			got = int(enc[0])
		} else {
			nBytes := int(enc[0] & 0x7F)
			for i := 1; i <= nBytes; i++ {
				got = (got << 8) | int(enc[i])
			}
		}
		if got != n {
			t.Errorf("n=%d encoded to %x, decoded back to %d", n, enc, got)
		}
	}
}

func TestWrapTLV_HighTagNumberForm(t *testing.T) {
	// Tag 54 (BoundProfilePackage outer): APPLICATION class,
	// constructed → first byte 0x7F. Tag VLQ for 54 is single byte 0x36.
	got := wrapTLV(54, classApplication, true, []byte{0xAA, 0xBB})
	want := []byte{0x7F, 0x36, 0x02, 0xAA, 0xBB}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestWrapTLV_ShortTagForm(t *testing.T) {
	// Tag 16 (initialiseSecureChannelRequest): context-specific,
	// constructed → first byte 0x80|0x20|0x10 = 0xB0... wait,
	// tag 16 = 0x10, 16 < 31 so short form.
	// 0x10 = 0001_0000, with class=10 (context) and constructed bit:
	// 1010_0000 | 1_0000 = 0xB0.
	got := wrapTLV(16, classContextSpecific, true, []byte{0x01})
	want := []byte{0xB0, 0x01, 0x01}
	if !bytes.Equal(got, want) {
		t.Errorf("got %x, want %x", got, want)
	}
}

func TestStripTag_PreservesBody(t *testing.T) {
	// Construct a known TLV: SEQUENCE { OCTET STRING 'hi' }.
	body := []byte{0x04, 0x02, 'h', 'i'}
	der := append([]byte{0x30, byte(len(body))}, body...)
	if got := stripTag(der); !bytes.Equal(got, body) {
		t.Errorf("got %x, want %x", got, body)
	}
}
