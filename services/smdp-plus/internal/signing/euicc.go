// EuiccSigned1 verification — the eUICC's half of the ES9+ auth
// handshake (SGP.22 §5.7.5 / §5.7.13).
//
// On authenticateClient, the LPA forwards what the eUICC signed:
//
//	euiccSigned1   — the SEQUENCE the eUICC signed
//	euiccSignature1 — ECDSA-SHA-256 over DER(euiccSigned1) by the
//	                  eUICC's leaf key
//	euiccCert      — the eUICC's leaf cert, EUM-issued
//	eumCert        — the EUM intermediate that issued the eUICC cert
//
// SM-DP+ side checks:
//   1. The cert chain euiccCert → eumCert → CI root verifies against
//      the trust store.
//   2. The signature verifies against euiccCert's public key over
//      SHA-256(DER(euiccSigned1)).
//   3. transactionId in euiccSigned1 matches the open session.
//   4. serverChallenge in euiccSigned1 matches the one this SM-DP+
//      issued in initiateAuthentication (replay defense).
//   5. serverAddress in euiccSigned1 matches our configured address.
//
// This file does (1), (2), and (5). Caller (the HTTP handler)
// enforces (3) and (4) since those need session-store access.

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

// EuiccSigned1 mirrors the SGP.22 §5.7.13 SEQUENCE that the eUICC
// signs as part of authenticateClient. EUICCInfo2 and CtxParams1
// carry rich substructures the SM-DP+ must accept verbatim; we
// keep them as raw ASN.1 so we can pass them through and (later)
// parse the fields the spec needs without a re-encoding round
// that might produce different bytes than the eUICC signed.
type EuiccSigned1 struct {
	TransactionID   []byte
	ServerAddress   string `asn1:"utf8"`
	ServerChallenge []byte
	EUICCInfo2      asn1.RawValue
	CtxParams1      asn1.RawValue
}

// MarshalDER returns the DER encoding. Used by tests; production
// code should NOT re-marshal a parsed EuiccSigned1 and verify
// against that — it must verify against the bytes-on-the-wire.
func (e EuiccSigned1) MarshalDER() ([]byte, error) {
	return asn1.Marshal(e)
}

// UnmarshalEuiccSigned1 parses raw DER. Trailing bytes are rejected.
func UnmarshalEuiccSigned1(b []byte) (*EuiccSigned1, error) {
	var out EuiccSigned1
	rest, err := asn1.Unmarshal(b, &out)
	if err != nil {
		return nil, fmt.Errorf("signing: EuiccSigned1 unmarshal: %w", err)
	}
	if len(rest) != 0 {
		return nil, errors.New("signing: trailing bytes after EuiccSigned1")
	}
	return &out, nil
}

// VerifyOptions configures eUICC chain + signature verification.
type VerifyOptions struct {
	Roots         *x509.CertPool // CI roots from certmgr
	Intermediates *x509.CertPool // EUM intermediates from certmgr (optional; per-call ones still added below)
	ServerAddress string         // expected serverAddress in EuiccSigned1
}

// VerifyResult carries the parsed pieces a caller may want for
// downstream session checks (transaction id, server challenge).
type VerifyResult struct {
	EuiccSigned1 *EuiccSigned1
	EuiccCert    *x509.Certificate
	EumCert      *x509.Certificate
}

// VerifyEuiccAuthenticate runs the full chain + signature + address
// check. The four byte slices are the on-the-wire DER pieces the
// LPA forwarded; the signature must be DER SEQUENCE { r, s } per
// SGP.22 §H.5.
//
// On any failure, returns an error with a human-readable reason and
// a nil result. Callers that need to distinguish failure types can
// switch on the error chain.
func VerifyEuiccAuthenticate(
	euiccSigned1DER []byte,
	euiccSignature1 []byte,
	euiccCertDER []byte,
	eumCertDER []byte,
	opts VerifyOptions,
) (*VerifyResult, error) {
	if opts.Roots == nil {
		return nil, errors.New("signing: VerifyOptions.Roots is required")
	}

	euiccCert, err := x509.ParseCertificate(euiccCertDER)
	if err != nil {
		return nil, fmt.Errorf("signing: parse euicc cert: %w", err)
	}
	eumCert, err := x509.ParseCertificate(eumCertDER)
	if err != nil {
		return nil, fmt.Errorf("signing: parse eum cert: %w", err)
	}

	intermediates := x509.NewCertPool()
	if opts.Intermediates != nil {
		intermediates = opts.Intermediates.Clone()
	}
	intermediates.AddCert(eumCert)

	if _, err := euiccCert.Verify(x509.VerifyOptions{
		Roots:         opts.Roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return nil, fmt.Errorf("signing: euicc cert chain does not verify: %w", err)
	}

	pub, ok := euiccCert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("signing: euicc cert public key is %T, want ECDSA", euiccCert.PublicKey)
	}

	digest := sha256.Sum256(euiccSigned1DER)
	var sig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(euiccSignature1, &sig); err != nil {
		return nil, fmt.Errorf("signing: parse euiccSignature1: %w", err)
	}
	if !ecdsa.Verify(pub, digest[:], sig.R, sig.S) {
		return nil, errors.New("signing: euiccSignature1 does not verify against euicc cert public key")
	}

	parsed, err := UnmarshalEuiccSigned1(euiccSigned1DER)
	if err != nil {
		return nil, err
	}
	if opts.ServerAddress != "" && parsed.ServerAddress != opts.ServerAddress {
		return nil, fmt.Errorf("signing: euiccSigned1.serverAddress %q does not match expected %q",
			parsed.ServerAddress, opts.ServerAddress)
	}

	return &VerifyResult{
		EuiccSigned1: parsed,
		EuiccCert:    euiccCert,
		EumCert:      eumCert,
	}, nil
}
