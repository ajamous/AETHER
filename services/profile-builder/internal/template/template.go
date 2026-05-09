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
// build step returns a JSON envelope until the codec lands.
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
		MCC        string `yaml:"mcc" json:"mcc"`
		MNC        string `yaml:"mnc" json:"mnc"`
		PLMNName  string `yaml:"plmn_name" json:"plmn_name"`
		HPLMNAct   string `yaml:"hplmn_act" json:"hplmn_act"`
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
	IMSI    string `json:"imsi"`
	ICCID   string `json:"iccid"`
	MSISDN  string `json:"msisdn"`
	Ki      []byte `json:"ki"`
	OPc     []byte `json:"opc"`
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

// UPPEnvelope is the JSON-shaped placeholder for a real SAIP UPP.
// It captures the inputs the SAIP codec will marshal once it lands;
// services downstream (smdp-plus) treat it as opaque, so swapping
// the body for ASN.1 bytes later is a non-breaking change.
type UPPEnvelope struct {
	Profile    *Profile        `json:"profile"`
	Subscriber *SubscriberData `json:"subscriber"`
	Note       string          `json:"_note"`
}

// BuildUPP merges p and s into a UPP envelope, validating both inputs.
//
// SAIP-faithful encoding lives in pkg/saip (planned). This function's
// signature does not change when that codec lands; only the returned
// bytes do.
func BuildUPP(p *Profile, s *SubscriberData) (*UPPEnvelope, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := ValidateSubscriber(s); err != nil {
		return nil, err
	}
	return &UPPEnvelope{
		Profile:    p,
		Subscriber: s,
		Note:       "JSON envelope — replaced with SAIP-encoded UPP when pkg/saip lands",
	}, nil
}
