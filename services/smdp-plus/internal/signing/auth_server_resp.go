// AuthenticateResponseOk — the SGP.22 §5.7.5 outer SEQUENCE the LPA
// forwards on authenticateClient.
//
// In a spec-faithful deployment the LPA forwards a single
// AuthenticateServerResponse blob (a CHOICE of Ok or Error). Until
// this codec landed, smdp-plus accepted the four signed pieces as
// individual JSON fields — useful as a lab seam but a wire shape
// no real LPA emits. This file ships the Ok-alternative SEQUENCE so
// the handler can parse the real blob; the explicit four-field path
// stays available as a lab fallback.
//
// Wire shape:
//
//	AuthenticateResponseOk ::= SEQUENCE {
//	    euiccSigned1     EuiccSigned1,           -- §5.7.13 SEQUENCE
//	    euiccSignature1  [APPLICATION 55] OCTET STRING,
//	    euiccCertificate Certificate,            -- X.509 SEQUENCE
//	    eumCertificate   Certificate             -- X.509 SEQUENCE
//	}
//
// All three SEQUENCE-shaped fields use asn1.RawValue so the bytes
// the eUICC signed and the X.509 DER the caller hands to
// x509.ParseCertificate survive a Marshal/Unmarshal round-trip
// unchanged. encoding/asn1 cannot re-marshal a parsed struct
// identically when optional fields are absent; preserving raw bytes
// is the only way the signature-over-the-signed-blob check stays
// valid.
//
// The CHOICE-of-Error alternative is not modeled — anything that
// does not parse as Ok is treated as malformed by the caller.

package signing

import (
	"encoding/asn1"
	"errors"
	"fmt"
)

// AuthenticateResponseOk is the SGP.22 §5.7.5 Ok-alternative payload.
type AuthenticateResponseOk struct {
	EuiccSigned1     asn1.RawValue
	EuiccSignature1  []byte `asn1:"tag:55,application"`
	EuiccCertificate asn1.RawValue
	EumCertificate   asn1.RawValue
}

// MarshalAuthenticateResponseOk builds the outer SEQUENCE from its
// four byte slices. Used by the lab test harness to forge LPA
// forwards; production-side smdp-plus only ever calls Unmarshal.
func MarshalAuthenticateResponseOk(euiccSigned1DER, euiccSignature1, euiccCertDER, eumCertDER []byte) ([]byte, error) {
	if len(euiccSigned1DER) == 0 {
		return nil, errors.New("signing: euiccSigned1DER required")
	}
	if len(euiccSignature1) == 0 {
		return nil, errors.New("signing: euiccSignature1 required")
	}
	if len(euiccCertDER) == 0 {
		return nil, errors.New("signing: euiccCertDER required")
	}
	if len(eumCertDER) == 0 {
		return nil, errors.New("signing: eumCertDER required")
	}
	wrapper := AuthenticateResponseOk{
		EuiccSigned1:     asn1.RawValue{FullBytes: euiccSigned1DER},
		EuiccSignature1:  euiccSignature1,
		EuiccCertificate: asn1.RawValue{FullBytes: euiccCertDER},
		EumCertificate:   asn1.RawValue{FullBytes: eumCertDER},
	}
	return asn1.Marshal(wrapper)
}

// UnmarshalAuthenticateResponseOk decodes the outer SEQUENCE and
// returns the four pieces. Caller then feeds them to
// VerifyEuiccAuthenticate exactly as if they had arrived as four
// explicit JSON fields.
func UnmarshalAuthenticateResponseOk(blob []byte) (euiccSigned1DER, euiccSignature1, euiccCertDER, eumCertDER []byte, err error) {
	var wrapper AuthenticateResponseOk
	rest, err := asn1.Unmarshal(blob, &wrapper)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("signing: AuthenticateResponseOk unmarshal: %w", err)
	}
	if len(rest) != 0 {
		return nil, nil, nil, nil, errors.New("signing: trailing bytes after AuthenticateResponseOk")
	}
	if len(wrapper.EuiccSigned1.FullBytes) == 0 {
		return nil, nil, nil, nil, errors.New("signing: AuthenticateResponseOk missing euiccSigned1")
	}
	if len(wrapper.EuiccSignature1) == 0 {
		return nil, nil, nil, nil, errors.New("signing: AuthenticateResponseOk missing euiccSignature1")
	}
	if len(wrapper.EuiccCertificate.FullBytes) == 0 {
		return nil, nil, nil, nil, errors.New("signing: AuthenticateResponseOk missing euiccCertificate")
	}
	if len(wrapper.EumCertificate.FullBytes) == 0 {
		return nil, nil, nil, nil, errors.New("signing: AuthenticateResponseOk missing eumCertificate")
	}
	return wrapper.EuiccSigned1.FullBytes,
		wrapper.EuiccSignature1,
		wrapper.EuiccCertificate.FullBytes,
		wrapper.EumCertificate.FullBytes,
		nil
}
