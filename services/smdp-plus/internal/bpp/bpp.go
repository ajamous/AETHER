// BoundProfilePackage assembly per SGP.22 §5.7.6.
//
// This file ships the outer ASN.1 wrapper that everything else in
// the BPP critical path produces inputs for:
//
//   - The SM-DP+'s ephemeral pubkey + DPpb signature go into
//     `InitialiseSecureChannelRequest`.
//   - The four `ControlRefTemplate` fields carry the SCP03t
//     parameters the eUICC needs before it can decrypt anything.
//   - `SealSegments` (segment.go) produces the
//     `sequenceOf88` body — the AES-128-GCM-sealed UPP segments.
//
// Wiring this into the `getBoundProfilePackage` HTTP handler is
// the explicit follow-up PR. This file ships only the codec so
// the wire-shape part can be reviewed and tested in isolation
// from the surrounding session-state and ephemeral-key plumbing.
//
// SGP.22 §5.7.6 wire shape:
//
//	BoundProfilePackage ::= [APPLICATION 54] SEQUENCE {
//	    initialiseSecureChannelRequest [16] InitialiseSecureChannelRequest,
//	    firstSequenceOf87  [APPLICATION 23] SEQUENCE OF [7] OCTET STRING OPTIONAL,
//	    sequenceOf88       [APPLICATION 24] SEQUENCE OF [8] OCTET STRING,
//	    secondSequenceOf87 [APPLICATION 25] SEQUENCE OF [7] OCTET STRING OPTIONAL,
//	    sequenceOf86       [APPLICATION 26] SEQUENCE OF [6] OCTET STRING
//	}
//
//	InitialiseSecureChannelRequest ::= [APPLICATION 35] SEQUENCE {
//	    remoteOpId          [0] INTEGER,
//	    transactionId       [1] OCTET STRING (SIZE(1..16)),
//	    controlRefTemplate  [APPLICATION 6] ControlRefTemplate,
//	    smdpOtpk            [APPLICATION 73] OCTET STRING,
//	    smdpSign            [APPLICATION 55] OCTET STRING
//	}
//
//	ControlRefTemplate ::= SEQUENCE {
//	    keyUsageQualifier [APPLICATION 47] OCTET STRING,
//	    keyType           [APPLICATION 48] OCTET STRING,
//	    keyLength         [APPLICATION 49] OCTET STRING,
//	    hostId            [APPLICATION 50] OCTET STRING OPTIONAL
//	}
//
// The encoder uses Go's `encoding/asn1` defaults plus per-field
// `asn1:"tag:N,class,..."` annotations for the APPLICATION-tagged
// fields the spec calls out.
package bpp

import (
	"encoding/asn1"
	"errors"
	"fmt"
)

// RemoteOpId per SGP.22 §5.7.6: 1 = installBoundProfilePackage.
// Only one operation is defined today; we expose the constant so
// callers don't sprinkle magic numbers.
const RemoteOpIdInstallBoundProfilePackage = 1

// SCP03t SGP.22 §H.4 ControlRefTemplate field constants. The
// values are 1-byte OCTET STRINGs the eUICC consumes when it sets
// up its half of the SCP03t channel.
//
// keyType        = 88 (AES with GCM, ES8+ default)
// keyLength      = 16 (AES-128)
// keyUsage       = c8 (encryption + integrity, AES-GCM combined mode)
// These match SGP.22 §H.4 Table H-1; if the spec adds new
// modes the constants gain new symbolic names without
// breaking existing callers.
var (
	KeyUsageQualifierEncryptAndIntegrity = []byte{0xC8}
	KeyTypeAESGCM                        = []byte{0x88}
	KeyLengthAES128                      = []byte{0x10}
)

// ControlRefTemplate is SGP.22 §H.4. Encoded with default Go ASN.1
// tags for the SEQUENCE wrapper plus APPLICATION-N OCTET STRINGs
// per field per the spec.
type ControlRefTemplate struct {
	KeyUsageQualifier []byte `asn1:"tag:47,application"`
	KeyType           []byte `asn1:"tag:48,application"`
	KeyLength         []byte `asn1:"tag:49,application"`
	HostID            []byte `asn1:"tag:50,application,optional"`
}

// InitialiseSecureChannelRequest is SGP.22 §5.7.7 — the signed
// preamble that opens the BPP. The eUICC verifies smdpSign
// against the SM-DP+'s DPpb cert chain BEFORE it processes any of
// the encrypted segments that follow.
type InitialiseSecureChannelRequest struct {
	RemoteOpId         int                `asn1:"tag:0"`
	TransactionID      []byte             `asn1:"tag:1"`
	ControlRefTemplate ControlRefTemplate `asn1:"tag:6,application"`
	SMDPOtpk           []byte             `asn1:"tag:73,application"`
	SMDPSign           []byte             `asn1:"tag:55,application"`
}

// SignedInputBytes returns the concatenation
// (transactionId || smdpOtpk || euiccOtpk) that the SM-DP+ MUST
// sign with its DPpb key per SGP.22 §5.7.7. The result is the
// pre-hash input — callers SHA-256 this and pass the digest to
// the HSM broker's Sign endpoint.
//
// We expose the function rather than baking it into the codec so
// callers see exactly what gets signed; auditors looking at this
// file can match the spec's signed-input definition without
// reading codec internals.
func SignedInputBytes(transactionID, smdpOtpk, euiccOtpk []byte) []byte {
	out := make([]byte, 0, len(transactionID)+len(smdpOtpk)+len(euiccOtpk))
	out = append(out, transactionID...)
	out = append(out, smdpOtpk...)
	out = append(out, euiccOtpk...)
	return out
}

// MarshalDER produces the DER encoding of the
// InitialiseSecureChannelRequest SEQUENCE. The SEQUENCE itself
// goes into `[16]` inside BoundProfilePackage; that wrapping is
// added by AssembleBoundProfilePackage, not here.
func (r InitialiseSecureChannelRequest) MarshalDER() ([]byte, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	return asn1.Marshal(r)
}

// UnmarshalInitialiseSecureChannelRequest parses DER bytes back to
// the struct. Used by tests today; an audit-replay tool would use
// this in production to confirm a captured BPP's preamble decodes
// to fields the operator expects.
func UnmarshalInitialiseSecureChannelRequest(b []byte) (*InitialiseSecureChannelRequest, error) {
	var out InitialiseSecureChannelRequest
	rest, err := asn1.Unmarshal(b, &out)
	if err != nil {
		return nil, fmt.Errorf("bpp: ISCR unmarshal: %w", err)
	}
	if len(rest) != 0 {
		return nil, errors.New("bpp: trailing bytes after ISCR")
	}
	return &out, nil
}

func (r InitialiseSecureChannelRequest) validate() error {
	if r.RemoteOpId < 0 || r.RemoteOpId > 99 {
		return fmt.Errorf("bpp: ISCR.RemoteOpId %d outside 0..99", r.RemoteOpId)
	}
	if l := len(r.TransactionID); l < 1 || l > 16 {
		return fmt.Errorf("bpp: ISCR.TransactionID length %d outside 1..16", l)
	}
	if n := len(r.SMDPOtpk); n != 0 && n != 33 && n != 65 {
		return fmt.Errorf("bpp: ISCR.SMDPOtpk length %d (want 33 compressed or 65 uncompressed)", n)
	}
	if len(r.SMDPSign) == 0 {
		return errors.New("bpp: ISCR.SMDPSign required")
	}
	return nil
}

// AssembleBoundProfilePackage builds the outer SGP.22 §5.7.6
// SEQUENCE around the signed preamble + the SCP03t-sealed UPP
// segments.
//
// `iscr` carries the InitialiseSecureChannelRequest the eUICC
// will verify first; `sequenceOf88Segments` is the list of GCM-
// sealed segment bytes from `SealSegments`. The two optional
// `firstSequenceOf87` / `secondSequenceOf87` slots and the
// trailing `sequenceOf86` slot stay empty in this minimum-viable
// shipping shape — they carry profile-payload-derived shrouded
// data the eUICC fills in during installation, not data the
// SM-DP+ originates.
//
// Returns the DER-encoded `[APPLICATION 54]` outer wrapper.
func AssembleBoundProfilePackage(iscr InitialiseSecureChannelRequest, sequenceOf88Segments [][]byte) ([]byte, error) {
	if len(sequenceOf88Segments) == 0 {
		return nil, errors.New("bpp: AssembleBoundProfilePackage: no sequenceOf88 segments")
	}
	iscrDER, err := iscr.MarshalDER()
	if err != nil {
		return nil, fmt.Errorf("bpp: assemble: ISCR: %w", err)
	}

	// boundProfilePackage is a hand-rolled SEQUENCE-of-tagged-
	// fields shape. We cannot use a flat Go struct here because
	// the trailing `sequenceOf86` field is mandatory but EMPTY in
	// our minimum-viable shipping shape — encoding/asn1 has no
	// concise way to express "include the tag with zero
	// children". We assemble bytes directly.
	var inner []byte

	// [16] initialiseSecureChannelRequest — the SEQUENCE bytes
	// from MarshalDER, re-tagged with [16] context-specific.
	inner = append(inner, retag(iscrDER, 16, classContextSpecific, true)...)

	// [APPLICATION 24] sequenceOf88: outer is APPLICATION-tagged
	// constructed; inner children are each [APPLICATION 24]
	// per SGP.22's "sequence of [8] OCTET STRING" but the [8]
	// here is APPLICATION-class per the spec table, so each
	// segment is wrapped in [APPLICATION 8] OCTET STRING.
	var seqOf88Body []byte
	for _, seg := range sequenceOf88Segments {
		seqOf88Body = append(seqOf88Body, wrapTLV(8, classApplication, false, seg)...)
	}
	inner = append(inner, wrapTLV(24, classApplication, true, seqOf88Body)...)

	// [APPLICATION 26] sequenceOf86: mandatory but EMPTY. The
	// eUICC accepts an empty SEQUENCE here for profile shapes
	// that don't need post-install payloads. A future PR can
	// populate this when those payloads are wired.
	inner = append(inner, wrapTLV(26, classApplication, true, nil)...)

	return wrapTLV(54, classApplication, true, inner), nil
}

// --- low-level TLV helpers ------------------------------------------------
//
// We hand-roll these rather than using asn1.Marshal for the outer
// wrappers because we need byte-exact control over the
// APPLICATION-tagged constructed forms with mixed-class children,
// which Go's encoding/asn1 doesn't express in a single struct.

const (
	classUniversal       byte = 0x00 // bits 7-6 = 00
	classApplication     byte = 0x40 // bits 7-6 = 01
	classContextSpecific byte = 0x80 // bits 7-6 = 10
)

// wrapTLV builds a single TLV with the given tag number, class,
// constructed bit, and body bytes.
//
// Tag numbers up to 30 use the short form (1-byte tag). Tag
// numbers ≥ 31 use the high-tag-number form (multi-byte tag with
// VLQ encoding). We use the high-tag-number form for tag 47, 54,
// 73, 88 etc. that the spec mandates.
func wrapTLV(tagNumber int, class byte, constructed bool, body []byte) []byte {
	if tagNumber < 0 {
		panic("wrapTLV: negative tag number")
	}

	var firstByte byte = class
	if constructed {
		firstByte |= 0x20
	}

	var tagBytes []byte
	if tagNumber < 31 {
		firstByte |= byte(tagNumber)
		tagBytes = []byte{firstByte}
	} else {
		// High-tag-number form: first byte's low 5 bits = 11111,
		// then VLQ-encode the tag number with continuation bits.
		firstByte |= 0x1F
		tagBytes = []byte{firstByte}
		// VLQ: most-significant 7-bit groups first, with bit 7
		// set on all but the last byte.
		var vlq []byte
		t := tagNumber
		for t > 0 {
			vlq = append([]byte{byte(t & 0x7F)}, vlq...)
			t >>= 7
		}
		for i := 0; i < len(vlq)-1; i++ {
			vlq[i] |= 0x80
		}
		tagBytes = append(tagBytes, vlq...)
	}

	out := make([]byte, 0, len(tagBytes)+5+len(body))
	out = append(out, tagBytes...)
	out = append(out, derLength(len(body))...)
	out = append(out, body...)
	return out
}

// retag strips the existing tag from a DER-encoded TLV and
// re-wraps it under the given (tagNumber, class, constructed).
// The body bytes are preserved unchanged.
func retag(der []byte, newTagNumber int, newClass byte, constructed bool) []byte {
	body := stripTag(der)
	return wrapTLV(newTagNumber, newClass, constructed, body)
}

// stripTag returns the value-bytes of a TLV (everything after the
// tag and length octets). Caller is responsible for handing in
// well-formed DER.
func stripTag(der []byte) []byte {
	if len(der) == 0 {
		return nil
	}
	// Skip the tag (handles short-form tag for our uses).
	off := 1
	if der[0]&0x1F == 0x1F {
		// High-tag-number form: skip continuation bytes.
		for off < len(der) && der[off]&0x80 != 0 {
			off++
		}
		off++
	}
	// Skip the length.
	if off >= len(der) {
		return nil
	}
	lengthByte := der[off]
	off++
	if lengthByte&0x80 != 0 {
		nLen := int(lengthByte & 0x7F)
		off += nLen
	}
	if off > len(der) {
		return nil
	}
	return der[off:]
}

// derLength encodes len(body) per X.690 §8.1.3.
func derLength(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	var lenBytes []byte
	tmp := n
	for tmp > 0 {
		lenBytes = append([]byte{byte(tmp & 0xFF)}, lenBytes...)
		tmp >>= 8
	}
	return append([]byte{0x80 | byte(len(lenBytes))}, lenBytes...)
}
