// Package saip implements the SGP.22 §B (TCA SAIP v2.x) Subscription
// Manager Application for Profile codec.
//
// Scope of this package today
//
// This is the minimum-viable subset of the SGP.22 SAIP profile
// package: enough to assemble + decode a syntactically valid
// ProfileHeader + end marker so that
//
//   - profile-builder can emit a real DER-encoded UPP byte sequence
//     (today it only emits a JSON envelope)
//   - smdp-plus can lift `GetBoundProfilePackage` from honest 501
//     to producing a real SAIP UPP wrapped in BSP / PPP / BPP
//
// The fuller catalogue — PE-USIM, PE-ISIM, PE-FileSystem,
// PE-AKAParameter, PE-PinCodes, PE-Application, PE-RFM,
// PE-SecurityDomain, etc. — is the explicit follow-up. Each lands
// as a separate type behind the same CHOICE wire shape, so adding
// them does not change how callers assemble a ProfilePackage.
//
// Why hand-rolled
//
// Aether's plan philosophy avoids GPL-encumbered upstream ASN.1
// modules. The SGP.22 spec text is the source of truth for these
// types; the Go declarations below are a direct transcription with
// the implicit/explicit tags called out per §B. A round-trip test
// per type is the contract.
//
// Wire format
//
// Encoding is DER per X.690 (`encoding/asn1` defaults). Every
// ProfileElement is tagged with its CHOICE alternative's
// context-specific tag — ProfileHeader is [0], PEEnd is [99]. The
// outer ProfilePackage is a SEQUENCE OF.
package saip

import (
	"bytes"
	"encoding/asn1"
	"errors"
	"fmt"
)

// SAIPMajorVersion / SAIPMinorVersion are the on-the-wire version
// stamps the spec requires in every ProfileHeader. The values
// here match TCA SAIP v2.3 / SGP.22 v3.x. Bumping them is the
// caller's choice, not this package's.
const (
	SAIPMajorVersion = 2
	SAIPMinorVersion = 3
)

// ProfileType strings the spec lists in B.1. We expose the most
// common ones as constants so callers don't typo. Operators with a
// custom `PROFILE_TYPE` agreed with their carrier just pass the
// string directly.
const (
	ProfileTypeGSMA = "GSMA Generic eUICC Test Profile"
	ProfileTypeSAS  = "SAS-SM Test Profile"
)

// ProfileHeader mirrors SGP.22 §B.1 ProfileHeader.
//
//	ProfileHeader ::= SEQUENCE {
//	    major-version             INTEGER,
//	    minor-version             INTEGER,
//	    profileType               UTF8String,
//	    iccid                     OCTET STRING (SIZE(10)),  -- nibble-swapped
//	    eUICC-Mandatory-services  SEQUENCE OF UTF8String
//	}
//
// PolicyRules and the mandatory-GFSTE-list field from the full
// spec are deliberately omitted in this minimum-viable subset;
// they land in a follow-up PR.
type ProfileHeader struct {
	MajorVersion             int
	MinorVersion             int
	ProfileType              string   `asn1:"utf8"`
	ICCID                    []byte   // 10 octets, nibble-swapped per SGP.22 §B.1
	EUICCMandatoryServices   []string `asn1:"sequence"`
}

// PEEnd is the SGP.22 §B.X end-of-package marker — an empty SEQUENCE.
// A ProfilePackage that lacks PEEnd is malformed; eUICCs reject it.
type PEEnd struct{}

// ProfilePackage is the top-level CHOICE-of-ProfileElement SEQUENCE
// OF the spec defines.
//
// Today we expose a typed helper (Build) rather than a raw struct
// so callers can't accidentally produce an invalid (e.g.
// no-PEEnd, or PEEnd-not-last) sequence. Callers that need direct
// element-by-element control can instead append pre-marshalled
// elements via AppendRaw.
type ProfilePackage struct {
	// elements holds DER-encoded ProfileElement bytes in order.
	// Each entry is already wrapped in its CHOICE tag — Build()
	// guarantees this.
	elements [][]byte
}

// Build constructs a ProfilePackage from a header and a list of
// additional elements (currently only PEEnd is supported as a
// concrete builder; richer types follow). It enforces:
//
//   - Header is present and validates.
//   - PEEnd terminates the package; nothing follows it.
//   - The ICCID is exactly 10 octets.
//
// Returns the assembled package; call MarshalDER to get bytes.
func Build(header ProfileHeader) (*ProfilePackage, error) {
	if err := header.validate(); err != nil {
		return nil, err
	}
	pkg := &ProfilePackage{}

	// ProfileElement.header is CHOICE alternative [0] — implicit
	// tag wrapping the SEQUENCE.
	hdrBytes, err := asn1.MarshalWithParams(header, "tag:0")
	if err != nil {
		return nil, fmt.Errorf("saip: marshal header: %w", err)
	}
	pkg.elements = append(pkg.elements, hdrBytes)

	// PEEnd is CHOICE alternative [99] — wrap an empty SEQUENCE.
	endBytes, err := asn1.MarshalWithParams(PEEnd{}, "tag:99")
	if err != nil {
		return nil, fmt.Errorf("saip: marshal end: %w", err)
	}
	pkg.elements = append(pkg.elements, endBytes)

	return pkg, nil
}

// AppendRaw splices a pre-marshalled ProfileElement into the
// package between the header and the PEEnd terminator. The bytes
// MUST already be CHOICE-tagged (i.e. produced by
// asn1.MarshalWithParams with the correct `tag:N`). Use this for
// element types this package does not yet provide a concrete
// builder for — RFM, application, etc.
//
// Calling AppendRaw after Build inserts the element BEFORE the
// PEEnd marker so the package stays valid.
func (p *ProfilePackage) AppendRaw(elementBytes []byte) error {
	if len(elementBytes) == 0 {
		return errors.New("saip: AppendRaw: empty element")
	}
	if len(p.elements) < 2 {
		return errors.New("saip: AppendRaw: package not built; call Build first")
	}
	// Insert before the trailing PEEnd.
	out := make([][]byte, 0, len(p.elements)+1)
	out = append(out, p.elements[:len(p.elements)-1]...)
	out = append(out, elementBytes)
	out = append(out, p.elements[len(p.elements)-1])
	p.elements = out
	return nil
}

// MarshalDER returns the DER encoding of the ProfilePackage as a
// SEQUENCE OF its CHOICE-tagged elements.
func (p *ProfilePackage) MarshalDER() ([]byte, error) {
	if len(p.elements) == 0 {
		return nil, errors.New("saip: empty ProfilePackage")
	}
	// Concatenate the children, then wrap in a SEQUENCE header.
	// We do this directly rather than via asn1.Marshal because Go's
	// encoding/asn1 doesn't support a `SEQUENCE OF CHOICE` shape
	// over a slice of pre-encoded byte buffers in a single call.
	body := bytes.Join(p.elements, nil)
	return wrapSEQUENCE(body), nil
}

// Decode parses a DER-encoded ProfilePackage. Returns the raw
// CHOICE-tagged element bytes in document order so callers can
// dispatch on the outer tag and decode the body themselves.
//
// Helpers for the well-known elements this package knows about
// (header, end) are exposed via DecodeHeader and IsEnd.
func Decode(b []byte) ([][]byte, error) {
	body, rest, err := unwrapSEQUENCE(b)
	if err != nil {
		return nil, fmt.Errorf("saip: outer SEQUENCE: %w", err)
	}
	if len(rest) != 0 {
		return nil, errors.New("saip: trailing bytes after ProfilePackage")
	}

	var out [][]byte
	for len(body) > 0 {
		// Walk one TLV at a time. We need the *full* TLV bytes
		// (including the outer tag/length) so the caller can
		// dispatch — strip and re-parse once we know the tag.
		_, _, n, err := peekTLV(body)
		if err != nil {
			return nil, fmt.Errorf("saip: element: %w", err)
		}
		out = append(out, append([]byte(nil), body[:n]...))
		body = body[n:]
	}
	return out, nil
}

// DecodeHeader parses an element previously returned by Decode if
// it is the [0] ProfileHeader CHOICE. Returns (header, true) on
// match, (zero, false) otherwise.
func DecodeHeader(elementBytes []byte) (ProfileHeader, bool) {
	tag, _, _, err := peekTLV(elementBytes)
	if err != nil {
		return ProfileHeader{}, false
	}
	if tag != 0 {
		return ProfileHeader{}, false
	}
	var hdr ProfileHeader
	if _, err := asn1.UnmarshalWithParams(elementBytes, &hdr, "tag:0"); err != nil {
		return ProfileHeader{}, false
	}
	return hdr, true
}

// IsEnd reports whether the element is the PEEnd terminator.
func IsEnd(elementBytes []byte) bool {
	tag, _, _, err := peekTLV(elementBytes)
	if err != nil {
		return false
	}
	return tag == 99
}

// --- validation -----------------------------------------------------------

func (h ProfileHeader) validate() error {
	if h.MajorVersion < 1 || h.MajorVersion > 99 {
		return fmt.Errorf("saip: ProfileHeader.MajorVersion %d outside 1..99", h.MajorVersion)
	}
	if h.MinorVersion < 0 || h.MinorVersion > 99 {
		return fmt.Errorf("saip: ProfileHeader.MinorVersion %d outside 0..99", h.MinorVersion)
	}
	if h.ProfileType == "" {
		return errors.New("saip: ProfileHeader.ProfileType required")
	}
	if len(h.ICCID) != 10 {
		return fmt.Errorf("saip: ICCID must be 10 octets (nibble-swapped), got %d", len(h.ICCID))
	}
	if len(h.EUICCMandatoryServices) == 0 {
		return errors.New("saip: ProfileHeader.EUICCMandatoryServices must list at least one service")
	}
	return nil
}

// --- low-level DER helpers ------------------------------------------------

// wrapSEQUENCE prepends a SEQUENCE (0x30) tag + DER length to body.
func wrapSEQUENCE(body []byte) []byte {
	out := []byte{0x30}
	out = append(out, derLength(len(body))...)
	out = append(out, body...)
	return out
}

// unwrapSEQUENCE strips the outer SEQUENCE tag and returns
// (body, rest-after-sequence, error). Rejects non-SEQUENCE inputs.
func unwrapSEQUENCE(b []byte) (body, rest []byte, err error) {
	if len(b) == 0 {
		return nil, nil, errors.New("empty input")
	}
	if b[0] != 0x30 {
		return nil, nil, fmt.Errorf("expected SEQUENCE tag 0x30, got 0x%02x", b[0])
	}
	bodyLen, headerLen, err := readDERLength(b[1:])
	if err != nil {
		return nil, nil, err
	}
	start := 1 + headerLen
	if len(b) < start+bodyLen {
		return nil, nil, errors.New("SEQUENCE length exceeds input")
	}
	return b[start : start+bodyLen], b[start+bodyLen:], nil
}

// peekTLV returns (contextSpecificTagNumber, body, totalTLVBytes, err)
// for the leading element. Only context-specific tags (class=2)
// are meaningful for ProfilePackage children, so we extract just
// the tag number.
func peekTLV(b []byte) (tag int, body []byte, n int, err error) {
	if len(b) == 0 {
		return 0, nil, 0, errors.New("empty TLV")
	}
	first := b[0]
	tagClass := (first & 0xC0) >> 6
	tagNumber := int(first & 0x1F)

	headerStart := 1
	if tagNumber == 0x1F {
		// Multi-byte tag (high tag number form). Decode VLQ.
		tagNumber = 0
		for {
			if headerStart >= len(b) {
				return 0, nil, 0, errors.New("truncated multi-byte tag")
			}
			c := b[headerStart]
			tagNumber = (tagNumber << 7) | int(c&0x7F)
			headerStart++
			if c&0x80 == 0 {
				break
			}
		}
	}
	if tagClass != 2 {
		// Not context-specific — caller may still want it; we
		// just return tagNumber as-is. Used for diagnostics only.
	}

	bodyLen, lenHeaderLen, err := readDERLength(b[headerStart:])
	if err != nil {
		return 0, nil, 0, err
	}
	bodyStart := headerStart + lenHeaderLen
	totalN := bodyStart + bodyLen
	if len(b) < totalN {
		return 0, nil, 0, errors.New("TLV length exceeds input")
	}
	return tagNumber, b[bodyStart:totalN], totalN, nil
}

// derLength encodes len(body) per X.690 §8.1.3.
func derLength(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	// Long form.
	var lenBytes []byte
	tmp := n
	for tmp > 0 {
		lenBytes = append([]byte{byte(tmp & 0xFF)}, lenBytes...)
		tmp >>= 8
	}
	out := []byte{0x80 | byte(len(lenBytes))}
	out = append(out, lenBytes...)
	return out
}

// readDERLength returns (body length, length-header byte count, error).
func readDERLength(b []byte) (int, int, error) {
	if len(b) == 0 {
		return 0, 0, errors.New("missing length octet")
	}
	first := b[0]
	if first < 0x80 {
		return int(first), 1, nil
	}
	nLenBytes := int(first & 0x7F)
	if nLenBytes == 0 {
		return 0, 0, errors.New("DER does not permit indefinite-length encoding")
	}
	if len(b) < 1+nLenBytes {
		return 0, 0, errors.New("truncated length octets")
	}
	v := 0
	for i := 0; i < nLenBytes; i++ {
		v = (v << 8) | int(b[1+i])
	}
	return v, 1 + nLenBytes, nil
}
