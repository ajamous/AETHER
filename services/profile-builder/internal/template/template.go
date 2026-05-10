// Package template loads, validates, and renders Aether's profile
// template format.
//
// Templates are YAML files under a configured directory. Each
// template is one carrier profile shape: IMSI, ICCID, network
// access app config, OTA keys, file system layout. The build step
// turns a template + per-subscriber data into a SAIP Unprotected
// Profile Package (UPP).
//
// Today the SAIP codec lives outside this package (planned under
// pkg/saip). This package handles template I/O and validation; the
// build step now produces real DER-encoded SAIP bytes via
// pkg/saip — minimum-viable header + PEEnd today, richer types
// land as pkg/saip grows.
package template

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ajamous/aether/pkg/saip"
)

// OTABytes returns the decoded OTA byte fields. Empty fields decode to nil.
func (p *Profile) OTABytes() (kic, kid, spi []byte, err error) {
	if kic, err = decodeOptionalB64(p.OTA.KIC); err != nil {
		return nil, nil, nil, fmt.Errorf("ota.kic: %w", err)
	}
	if kid, err = decodeOptionalB64(p.OTA.KID); err != nil {
		return nil, nil, nil, fmt.Errorf("ota.kid: %w", err)
	}
	if spi, err = decodeOptionalB64(p.OTA.SPI); err != nil {
		return nil, nil, nil, fmt.Errorf("ota.spi: %w", err)
	}
	return kic, kid, spi, nil
}

func decodeOptionalB64(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

// Profile is a parsed YAML profile template.
type Profile struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Version     string `yaml:"version" json:"version"`

	Network struct {
		MCC      string `yaml:"mcc" json:"mcc"`
		MNC      string `yaml:"mnc" json:"mnc"`
		PLMNName string `yaml:"plmn_name" json:"plmn_name"`
		HPLMNAct string `yaml:"hplmn_act" json:"hplmn_act"`
	} `yaml:"network" json:"network"`

	NAA struct {
		// USIM, ISIM, CSIM. SGP.22 / 3GPP TS 31.102.
		Apps []string `yaml:"apps" json:"apps"`
	} `yaml:"naa" json:"naa"`

	OTA struct {
		// Base64-encoded. Decoded by OTABytes() for use.
		KIC string `yaml:"kic" json:"kic"`
		KID string `yaml:"kid" json:"kid"`
		// SPI per ETSI TS 102.225
		SPI string `yaml:"spi" json:"spi"`
	} `yaml:"ota" json:"ota"`
}

// SubscriberData is the per-activation data merged with a Profile to
// produce a UPP.
type SubscriberData struct {
	IMSI   string `json:"imsi"`
	ICCID  string `json:"iccid"`
	MSISDN string `json:"msisdn"`
	Ki     []byte `json:"ki"`
	OPc    []byte `json:"opc"`
}

// Loader reads templates from a directory.
type Loader struct {
	dir string
}

// NewLoader returns a Loader rooted at dir.
func NewLoader(dir string) *Loader { return &Loader{dir: dir} }

// List returns the sorted set of template names available.
func (l *Loader) List() ([]string, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			out = append(out, strings.TrimSuffix(strings.TrimSuffix(name, ".yml"), ".yaml"))
		}
	}
	sort.Strings(out)
	return out, nil
}

// Load reads the named template.
func (l *Loader) Load(name string) (*Profile, error) {
	path, err := l.resolve(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	if p.Name == "" {
		p.Name = name
	}
	return p, nil
}

func (l *Loader) resolve(name string) (string, error) {
	for _, ext := range []string{".yaml", ".yml"} {
		p := filepath.Join(l.dir, name+ext)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("template %q not found in %s", name, l.dir)
}

// Validation rules used both by template load and per-subscriber merge.

var (
	imsiPattern  = regexp.MustCompile(`^\d{15}$`)
	iccidPattern = regexp.MustCompile(`^\d{19,20}$`)
	mccPattern   = regexp.MustCompile(`^\d{3}$`)
	mncPattern   = regexp.MustCompile(`^\d{2,3}$`)
)

// Validate checks that p is internally consistent.
func (p *Profile) Validate() error {
	if p.Name == "" {
		return errors.New("profile: name is required")
	}
	if !mccPattern.MatchString(p.Network.MCC) {
		return fmt.Errorf("profile: mcc %q must be 3 digits", p.Network.MCC)
	}
	if !mncPattern.MatchString(p.Network.MNC) {
		return fmt.Errorf("profile: mnc %q must be 2-3 digits", p.Network.MNC)
	}
	if len(p.NAA.Apps) == 0 {
		return errors.New("profile: at least one NAA app required")
	}
	for _, app := range p.NAA.Apps {
		switch strings.ToUpper(app) {
		case "USIM", "ISIM", "CSIM":
		default:
			return fmt.Errorf("profile: unknown NAA app %q", app)
		}
	}
	return nil
}

// ValidateSubscriber checks that a SubscriberData record is complete.
func ValidateSubscriber(s *SubscriberData) error {
	if !imsiPattern.MatchString(s.IMSI) {
		return fmt.Errorf("subscriber: imsi %q must be 15 digits", s.IMSI)
	}
	if !iccidPattern.MatchString(s.ICCID) {
		return fmt.Errorf("subscriber: iccid %q must be 19-20 digits", s.ICCID)
	}
	if len(s.Ki) != 16 {
		return fmt.Errorf("subscriber: ki must be 16 bytes, got %d", len(s.Ki))
	}
	if len(s.OPc) != 16 {
		return fmt.Errorf("subscriber: opc must be 16 bytes, got %d", len(s.OPc))
	}
	return nil
}

// UPPEnvelope wraps a SAIP-encoded UPP plus the inputs that
// produced it. The SAIP bytes are the wire artifact downstream
// services (smdp-plus) consume; the Profile + Subscriber fields
// are kept for human-readable inspection through the admin UI.
//
// SAIP today is the minimum-viable subset shipped by pkg/saip:
// ProfileHeader + PEEnd. Richer ProfileElements (PE-USIM,
// PE-PinCodes, etc.) land as pkg/saip grows; their bytes will
// appear inside SAIP without changing this envelope's shape.
type UPPEnvelope struct {
	Profile    *Profile        `json:"profile"`
	Subscriber *SubscriberData `json:"subscriber"`
	// SAIP is the DER-encoded ProfilePackage. Base64-encoded in
	// JSON so operators inspecting through the admin UI see a
	// stable string.
	SAIP []byte `json:"saip_der"`
	// Note carries any human-readable caveat about the encoding
	// (e.g. "minimum-viable subset" while pkg/saip is incomplete).
	Note string `json:"_note,omitempty"`
}

// BuildUPP merges p and s into a UPP envelope, validating both
// inputs and emitting a real DER-encoded SAIP ProfilePackage via
// pkg/saip. Today's package only encodes the header + PEEnd; the
// envelope's `saip_der` field is short by design until richer
// ProfileElements land in pkg/saip.
func BuildUPP(p *Profile, s *SubscriberData) (*UPPEnvelope, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := ValidateSubscriber(s); err != nil {
		return nil, err
	}

	iccidBytes, err := encodeICCIDNibbleSwapped(s.ICCID)
	if err != nil {
		return nil, fmt.Errorf("upp: %w", err)
	}
	hdr := saip.ProfileHeader{
		MajorVersion: saip.SAIPMajorVersion,
		MinorVersion: saip.SAIPMinorVersion,
		ProfileType:  profileTypeFor(p),
		ICCID:        iccidBytes,
		// Default to the GSMA mandatory-services baseline; profiles
		// that need more list them in their own template field once
		// the schema grows.
		EUICCMandatoryServices: []string{"contactless"},
	}
	pkg, err := saip.Build(hdr)
	if err != nil {
		return nil, fmt.Errorf("upp: saip build: %w", err)
	}
	der, err := pkg.MarshalDER()
	if err != nil {
		return nil, fmt.Errorf("upp: saip marshal: %w", err)
	}

	return &UPPEnvelope{
		Profile:    p,
		Subscriber: s,
		SAIP:       der,
		Note:       "SAIP minimum-viable subset (header + PEEnd); richer ProfileElements land as pkg/saip grows",
	}, nil
}

// profileTypeFor returns the SGP.22 profileType string. We use
// the template's `name` so operators see their template name in
// any decoded SAIP; fallback to the GSMA generic test string if
// the template is unnamed (validate() should have caught this
// already, defensive).
func profileTypeFor(p *Profile) string {
	if p.Name != "" {
		return p.Name
	}
	return saip.ProfileTypeGSMA
}

// encodeICCIDNibbleSwapped converts a 19- or 20-digit decimal
// ICCID string into the 10-octet nibble-swapped BCD form SGP.22
// §B.1 requires.
//
// For each pair (d1, d2) of decimal digits, the resulting octet
// is 0x[d2][d1] — i.e. low nibble carries the first digit, high
// nibble carries the second. A 19-digit ICCID is right-padded
// with the BCD pad nibble 0xF.
func encodeICCIDNibbleSwapped(iccid string) ([]byte, error) {
	if len(iccid) != 19 && len(iccid) != 20 {
		return nil, fmt.Errorf("iccid %q must be 19 or 20 digits, got %d", iccid, len(iccid))
	}
	// Pad to 20 with the BCD pad-nibble.
	padded := iccid
	if len(padded) == 19 {
		padded += "F"
	}
	out := make([]byte, 10)
	for i := 0; i < 10; i++ {
		hi := digitToNibble(padded[2*i+1])
		lo := digitToNibble(padded[2*i])
		if hi == 0xFF || lo == 0xFF {
			return nil, fmt.Errorf("iccid %q contains non-digit at offset %d", iccid, 2*i)
		}
		out[i] = (hi << 4) | lo
	}
	return out, nil
}

func digitToNibble(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c == 'F' || c == 'f':
		return 0x0F
	default:
		return 0xFF
	}
}
