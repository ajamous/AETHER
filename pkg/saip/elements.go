package saip

// Credential-carrying ProfileElements: PE-USIM (subscriber identity)
// and PE-AKAParameter (the long-term authentication keys).
//
// # Why these two next
//
// The header-only ProfilePackage that Build produces is
// syntactically valid but carries no subscriber credentials — an
// eUICC that installed it would hold a profile that cannot attach
// to any network. PE-USIM carries the IMSI + PLMN identity the
// network uses to recognise the subscriber; PE-AKAParameter carries
// the Milenage Ki + OPc the eUICC uses to answer the network's
// authentication challenge. Together they are the minimum a profile
// needs before "download succeeded" also means "the SIM works".
//
// # Fidelity, honestly
//
// These types follow the same pragmatic contract the rest of this
// package uses: each is a flat Go struct marshalled under its CHOICE
// alternative's context tag, and the contract a test locks is a
// DER round-trip, NOT cross-vendor interop. The precise TCA SAIP
// §B substructure — the PE-Header wrapper each element carries, the
// EF-IMSI packed-BCD layout in 3GPP TS 31.102 §4.2.2, the
// EF-AKA / Milenage OPc-vs-OP distinction, the templateID OBJECT
// IDENTIFIER — is the documented hardware-bench follow-up, same as
// the per-segment AAD layout in services/smdp-plus/internal/bpp/
// segment.go. The fields below carry the credential VALUES
// faithfully; aligning their wire framing to what a sysmoEUICC
// accepts lands when that bench is online.
//
// CHOICE tag numbers follow this package's existing pragmatic
// numbering (header [0], end [99]); they are distinct from those
// two and from each other, which is all the encoder/decoder here
// relies on.

import (
	"encoding/asn1"
	"fmt"
)

// CHOICE alternative tags for the elements this package builds. See
// the file godoc on fidelity: these are stable within Aether but the
// TCA §B alignment pass may renumber them.
const (
	tagUSIM         = 11
	tagAKAParameter = 5
)

// AKA algorithm identifiers. Milenage (3GPP TS 35.205/35.206) is the
// near-universal choice for USIM authentication; TUAK (TS 35.231) is
// reserved for when a deployment needs it.
const (
	AKAAlgorithmMilenage = 1
	AKAAlgorithmTUAK     = 2
)

// MilenageKeyLen is the byte length of the Milenage Ki and OPc.
const MilenageKeyLen = 16

// PEUSIM carries the USIM subscriber identity: the IMSI the network
// authenticates and the home PLMN it belongs to. 3GPP TS 31.102
// stores these across EF-IMSI / EF-AD; this struct carries the
// logical values, with the EF packing deferred per the file godoc.
type PEUSIM struct {
	IMSI string `asn1:"utf8"` // 5..15 decimal digits
	MCC  string `asn1:"utf8"` // 3 decimal digits
	MNC  string `asn1:"utf8"` // 2..3 decimal digits
}

// PEAKAParameter carries the Milenage authentication keys. Ki is the
// subscriber's long-term secret; OPc is the operator-variant
// algorithm configuration field (the per-subscriber OPc, not the
// network-wide OP). Both are 16 bytes for Milenage.
type PEAKAParameter struct {
	AlgorithmID int
	Ki          []byte
	OPc         []byte
}

// BuildUSIM marshals a PE-USIM element to its CHOICE-tagged DER. The
// result is ready to splice into a ProfilePackage via AppendRaw.
func BuildUSIM(u PEUSIM) ([]byte, error) {
	if err := u.validate(); err != nil {
		return nil, err
	}
	b, err := asn1.MarshalWithParams(u, fmt.Sprintf("tag:%d", tagUSIM))
	if err != nil {
		return nil, fmt.Errorf("saip: marshal PE-USIM: %w", err)
	}
	return b, nil
}

// BuildAKAParameter marshals a PE-AKAParameter element to its
// CHOICE-tagged DER, ready for AppendRaw.
func BuildAKAParameter(a PEAKAParameter) ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	b, err := asn1.MarshalWithParams(a, fmt.Sprintf("tag:%d", tagAKAParameter))
	if err != nil {
		return nil, fmt.Errorf("saip: marshal PE-AKAParameter: %w", err)
	}
	return b, nil
}

// DecodeUSIM parses an element previously returned by Decode if it
// is the [tagUSIM] PE-USIM CHOICE. Returns (usim, true) on match.
func DecodeUSIM(elementBytes []byte) (PEUSIM, bool) {
	tag, _, err := peekTLV(elementBytes)
	if err != nil || tag != tagUSIM {
		return PEUSIM{}, false
	}
	var u PEUSIM
	if _, err := asn1.UnmarshalWithParams(elementBytes, &u, fmt.Sprintf("tag:%d", tagUSIM)); err != nil {
		return PEUSIM{}, false
	}
	return u, true
}

// DecodeAKAParameter parses an element previously returned by Decode
// if it is the [tagAKAParameter] PE-AKAParameter CHOICE.
func DecodeAKAParameter(elementBytes []byte) (PEAKAParameter, bool) {
	tag, _, err := peekTLV(elementBytes)
	if err != nil || tag != tagAKAParameter {
		return PEAKAParameter{}, false
	}
	var a PEAKAParameter
	if _, err := asn1.UnmarshalWithParams(elementBytes, &a, fmt.Sprintf("tag:%d", tagAKAParameter)); err != nil {
		return PEAKAParameter{}, false
	}
	return a, true
}

// --- validation -----------------------------------------------------------

func (u PEUSIM) validate() error {
	if !isDigits(u.IMSI, 5, 15) {
		return fmt.Errorf("saip: PE-USIM IMSI %q must be 5..15 decimal digits", u.IMSI)
	}
	if !isDigits(u.MCC, 3, 3) {
		return fmt.Errorf("saip: PE-USIM MCC %q must be 3 decimal digits", u.MCC)
	}
	if !isDigits(u.MNC, 2, 3) {
		return fmt.Errorf("saip: PE-USIM MNC %q must be 2..3 decimal digits", u.MNC)
	}
	return nil
}

func (a PEAKAParameter) validate() error {
	switch a.AlgorithmID {
	case AKAAlgorithmMilenage, AKAAlgorithmTUAK:
	default:
		return fmt.Errorf("saip: PE-AKAParameter unknown AlgorithmID %d", a.AlgorithmID)
	}
	if len(a.Ki) != MilenageKeyLen {
		return fmt.Errorf("saip: PE-AKAParameter Ki must be %d bytes, got %d", MilenageKeyLen, len(a.Ki))
	}
	if len(a.OPc) != MilenageKeyLen {
		return fmt.Errorf("saip: PE-AKAParameter OPc must be %d bytes, got %d", MilenageKeyLen, len(a.OPc))
	}
	return nil
}

// isDigits reports whether s is between minLen and maxLen characters
// and contains only ASCII decimal digits.
func isDigits(s string, minLen, maxLen int) bool {
	if len(s) < minLen || len(s) > maxLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
