// PrepareDownloadResponse verification — the eUICC's signed reply to
// SmdpSigned2 in the ES9+ §5.7.7 / §5.6.4 flow.
//
// At getBoundProfilePackage time the LPA forwards the eUICC's
// PrepareDownloadResponseOk to the SM-DP+:
//
//	euiccSigned2     — SEQUENCE the eUICC signed
//	euiccSignature2  — ECDSA-SHA-256 over DER(euiccSigned2), by the
//	                   eUICC's leaf key
//
// EuiccSigned2 carries the transactionId binding the message to the
// open session and the euicc's one-time public key (euiccOtpk) that
// the SM-DP+ uses for ECKA. Verifying the signature against the
// eUICC certificate captured at authenticateClient time and matching
// the transactionId are the spec-mandated checks before the SM-DP+
// trusts the otpk for key agreement.
//
// Until this verifier lands, smdp-plus took the otpk directly on the
// HTTP request (in-tree convenience documented on the type). With it
// landed, the in-tree path still accepts a raw otpk for the lab flow,
// but a request carrying a signed PrepareDownloadResponse is verified
// end to end before its otpk is used.
//
// Wire shape: this package models PrepareDownloadResponseOk as a
// SEQUENCE { euiccSigned2 SEQUENCE, euiccSignature2 OCTET STRING }.
// SGP.22 also defines an error CHOICE alternative we do not model
// (we treat anything that does not parse as Ok as malformed).
package signing

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
)

// EuiccSigned2 mirrors SGP.22 §5.7.7 EUICCSigned2. The
// APPLICATION-73 tag on EuiccOtpk follows the same convention as
// SmdpSigned2.BPPEuiccOtpk so encoding/asn1 emits the spec byte.
type EuiccSigned2 struct {
	TransactionID []byte
	EuiccOtpk     []byte `asn1:"tag:73,application"`
	// HashCC is the SHA-256 of a confirmation code when the operator
	// requires one. Optional; we leave it as a free-form OCTET
	// STRING here, validated by length only.
	HashCC []byte `asn1:"optional"`
}

// MarshalDER returns the DER encoding of the EuiccSigned2 SEQUENCE.
func (e EuiccSigned2) MarshalDER() ([]byte, error) {
	if err := e.validate(); err != nil {
		return nil, err
	}
	return asn1.Marshal(e)
}

// UnmarshalEuiccSigned2 decodes a DER-encoded EuiccSigned2 SEQUENCE.
// Trailing bytes are rejected.
func UnmarshalEuiccSigned2(b []byte) (*EuiccSigned2, error) {
	var out EuiccSigned2
	rest, err := asn1.Unmarshal(b, &out)
	if err != nil {
		return nil, fmt.Errorf("signing: EuiccSigned2 unmarshal: %w", err)
	}
	if len(rest) != 0 {
		return nil, errors.New("signing: trailing bytes after EuiccSigned2")
	}
	return &out, nil
}

func (e EuiccSigned2) validate() error {
	if l := len(e.TransactionID); l < 1 || l > 16 {
		return fmt.Errorf("signing: EuiccSigned2.transactionId length %d outside 1..16", l)
	}
	switch n := len(e.EuiccOtpk); n {
	case 33:
		if first := e.EuiccOtpk[0]; first != 0x02 && first != 0x03 {
			return fmt.Errorf("signing: EuiccSigned2.euiccOtpk compressed first byte 0x%02x", first)
		}
	case 65:
		if e.EuiccOtpk[0] != 0x04 {
			return fmt.Errorf("signing: EuiccSigned2.euiccOtpk uncompressed first byte 0x%02x", e.EuiccOtpk[0])
		}
	default:
		return fmt.Errorf("signing: EuiccSigned2.euiccOtpk length %d (want 33 compressed or 65 uncompressed)", n)
	}
	if n := len(e.HashCC); n != 0 && n != 32 {
		return fmt.Errorf("signing: EuiccSigned2.hashCC length %d (want 0 absent or 32 SHA-256)", n)
	}
	return nil
}

// PrepareDownloadResponseOk is the outer SEQUENCE the LPA forwards.
// Using asn1.RawValue for EuiccSigned2 preserves the exact DER bytes
// the eUICC signed — Unmarshal-then-re-Marshal of a struct would
// drift on optional-field absence and break signature verification.
type PrepareDownloadResponseOk struct {
	EuiccSigned2    asn1.RawValue
	EuiccSignature2 []byte
}

// MarshalDER builds a PrepareDownloadResponseOk from an EuiccSigned2
// payload and a signature over its DER bytes. The lab-side test
// harness uses this to forge LPA forwards; production-side smdp-plus
// only ever calls Unmarshal.
func MarshalPrepareDownloadResponseOk(euiccSigned2 EuiccSigned2, signature []byte) ([]byte, error) {
	signedDER, err := euiccSigned2.MarshalDER()
	if err != nil {
		return nil, err
	}
	wrapper := PrepareDownloadResponseOk{
		EuiccSigned2:    asn1.RawValue{FullBytes: signedDER},
		EuiccSignature2: signature,
	}
	return asn1.Marshal(wrapper)
}

// UnmarshalPrepareDownloadResponseOk decodes the outer SEQUENCE,
// returning the EuiccSigned2 DER bytes and the signature for the
// caller to verify.
func UnmarshalPrepareDownloadResponseOk(b []byte) (euiccSigned2DER, signature []byte, err error) {
	var wrapper PrepareDownloadResponseOk
	rest, err := asn1.Unmarshal(b, &wrapper)
	if err != nil {
		return nil, nil, fmt.Errorf("signing: PrepareDownloadResponseOk unmarshal: %w", err)
	}
	if len(rest) != 0 {
		return nil, nil, errors.New("signing: trailing bytes after PrepareDownloadResponseOk")
	}
	if len(wrapper.EuiccSigned2.FullBytes) == 0 {
		return nil, nil, errors.New("signing: PrepareDownloadResponseOk missing EuiccSigned2")
	}
	if len(wrapper.EuiccSignature2) == 0 {
		return nil, nil, errors.New("signing: PrepareDownloadResponseOk missing EuiccSignature2")
	}
	return wrapper.EuiccSigned2.FullBytes, wrapper.EuiccSignature2, nil
}

// PreparedDownload is the result of a successful PrepareDownload
// Response verification: the parsed EuiccSigned2 fields the SM-DP+
// then uses (the otpk for ECKA, the transactionId for session
// binding).
type PreparedDownload struct {
	EuiccOtpk     []byte
	TransactionID []byte
}

// VerifyPrepareDownloadResponse parses the LPA-forwarded blob,
// verifies the signature against euiccCertDER's public key over
// SHA-256(DER(EuiccSigned2)), and confirms transactionId matches the
// expected session. Returns the verified fields.
//
// euiccCertDER MUST be the same cert captured at authenticateClient
// (the SM-DP+ stashes it on the session). expectedTxID MUST be the
// session's transactionId — without this check a replay against a
// different session would succeed.
func VerifyPrepareDownloadResponse(blob, euiccCertDER, expectedTxID []byte) (*PreparedDownload, error) {
	if len(euiccCertDER) == 0 {
		return nil, errors.New("signing: VerifyPrepareDownloadResponse: empty euicc cert (session never captured one)")
	}
	signedDER, sig, err := UnmarshalPrepareDownloadResponseOk(blob)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(euiccCertDER)
	if err != nil {
		return nil, fmt.Errorf("signing: parse session euicc cert: %w", err)
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("signing: euicc cert public key is %T, want ECDSA", cert.PublicKey)
	}

	var ecdsaSig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sig, &ecdsaSig); err != nil {
		return nil, fmt.Errorf("signing: parse euiccSignature2: %w", err)
	}
	digest := sha256.Sum256(signedDER)
	if !ecdsa.Verify(pub, digest[:], ecdsaSig.R, ecdsaSig.S) {
		return nil, errors.New("signing: euiccSignature2 does not verify against the session's eUICC cert")
	}

	parsed, err := UnmarshalEuiccSigned2(signedDER)
	if err != nil {
		return nil, err
	}
	if !equalBytes(parsed.TransactionID, expectedTxID) {
		return nil, errors.New("signing: EuiccSigned2.transactionId does not match the session")
	}
	return &PreparedDownload{
		EuiccOtpk:     parsed.EuiccOtpk,
		TransactionID: parsed.TransactionID,
	}, nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
