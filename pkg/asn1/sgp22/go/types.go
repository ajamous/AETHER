package sgp22

import (
	"encoding/asn1"
	"errors"
	"fmt"
)

// PEHeader is a small example type used to validate the codec
// scaffolding. It is shaped after the recurring "header" pattern that
// appears across SGP.22 Profile Element TLVs (see SGP.22 §2.5 and the
// PE-Header definition referenced from Annex H), but it is not
// itself a faithful 1:1 replica of any specific spec type. We use it
// only to exercise round-trip encode/decode in tests until the real
// modules are vendored under ../modules/ and their Go types land
// here in Phase 1.
//
// Once the real Annex B modules are vendored, this type will be
// replaced with the spec-faithful definition and any callers
// updated. Tests will continue to enforce round-trip stability.
type PEHeader struct {
	MandatoryFlag bool   `asn1:"explicit,tag:0"`
	IccidPresent  bool   `asn1:"explicit,tag:1"`
	Identifier    []byte `asn1:"explicit,tag:2"`
}

// Marshal returns the DER encoding of h.
func (h PEHeader) Marshal() ([]byte, error) {
	return asn1.Marshal(h)
}

// UnmarshalPEHeader parses a DER-encoded PEHeader.
func UnmarshalPEHeader(b []byte) (PEHeader, error) {
	var h PEHeader
	rest, err := asn1.Unmarshal(b, &h)
	if err != nil {
		return PEHeader{}, fmt.Errorf("sgp22: PEHeader unmarshal: %w", err)
	}
	if len(rest) != 0 {
		return PEHeader{}, errors.New("sgp22: trailing bytes after PEHeader")
	}
	return h, nil
}
