// SmdpSigned2 — SGP.22 §5.7.14.
//
// The SM-DP+ signs SmdpSigned2 as part of PrepareDownloadResponse;
// the eUICC verifies the signature against the SM-DP+'s DPpb cert
// chain, then uses the contained `bppEuiccOtpk` reservation
// signal to decide whether to start its own ephemeral keypair
// generation for the upcoming BPP exchange.
//
// This file ships the codec + sign helper. Wiring SmdpSigned2 into
// a `prepareDownload` HTTP handler — and the matching session
// state that captures the eUICC's `otPK.EUICC.ECKA` returned in
// the eUICC's PrepareDownloadResponse — is the explicit follow-up
// PR that lifts smdp-plus from "BPP returns 501" toward producing
// a real BPP. SmdpSigned2 is the smallest piece of that flow that
// can be shipped on its own.

package signing

import (
	"context"
	"crypto/sha256"
	"encoding/asn1"
	"errors"
	"fmt"

	"github.com/ajamous/aether/pkg/hsmclient"
)

// SmdpSigned2 mirrors SGP.22 §5.7.14:
//
//	SmdpSigned2 ::= SEQUENCE {
//	    transactionId   TransactionId,                       -- OCTET STRING (SIZE(1..16))
//	    ccRequiredFlag  BOOLEAN,
//	    bppEuiccOtpk    [APPLICATION 73] OCTET STRING OPTIONAL
//	}
//
// `ccRequiredFlag` tells the eUICC whether the SM-DP+ requires a
// confirmation code on this session — the eUICC surfaces a UI
// prompt to the user when true.
//
// `bppEuiccOtpk` is OPTIONAL on the wire. When present, it
// reserves the eUICC's ephemeral public-key slot for the BPP
// exchange that follows; today's smdp-plus does not yet drive a
// PrepareDownload session, so the field is OPTIONAL in this
// struct and omitted on Marshal when the slice is nil.
//
// The APPLICATION-73 tag on bppEuiccOtpk is a SGP.22 quirk —
// every other field uses the default UNIVERSAL tag. We declare
// it via an asn1 struct tag so encoding/asn1 emits the right
// outer-tag bytes.
type SmdpSigned2 struct {
	TransactionID  []byte
	CCRequiredFlag bool
	BPPEuiccOtpk   []byte `asn1:"tag:73,application,optional"`
}

// MarshalDER returns the DER encoding of the SmdpSigned2
// SEQUENCE.
func (s SmdpSigned2) MarshalDER() ([]byte, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	return asn1.Marshal(s)
}

// UnmarshalSmdpSigned2 decodes a DER-encoded SmdpSigned2.
// Trailing bytes are rejected.
func UnmarshalSmdpSigned2(b []byte) (*SmdpSigned2, error) {
	var out SmdpSigned2
	rest, err := asn1.Unmarshal(b, &out)
	if err != nil {
		return nil, fmt.Errorf("signing: SmdpSigned2 unmarshal: %w", err)
	}
	if len(rest) != 0 {
		return nil, errors.New("signing: trailing bytes after SmdpSigned2")
	}
	return &out, nil
}

func (s SmdpSigned2) validate() error {
	if l := len(s.TransactionID); l < 1 || l > 16 {
		return fmt.Errorf("signing: SmdpSigned2.transactionId length %d outside spec range 1..16", l)
	}
	// bppEuiccOtpk, when present, is the X9.63 uncompressed point
	// for a P-256 public key — 0x04 || X || Y, 65 bytes total.
	// Some implementations send the compressed form (33 bytes,
	// 0x02/0x03 prefix). Reject anything else.
	if n := len(s.BPPEuiccOtpk); n != 0 && n != 33 && n != 65 {
		return fmt.Errorf("signing: bppEuiccOtpk length %d (expected 33 compressed, 65 uncompressed, or 0 for absent)", n)
	}
	if n := len(s.BPPEuiccOtpk); n == 33 {
		first := s.BPPEuiccOtpk[0]
		if first != 0x02 && first != 0x03 {
			return fmt.Errorf("signing: bppEuiccOtpk compressed-point first byte 0x%02x (expected 0x02 or 0x03)", first)
		}
	}
	if n := len(s.BPPEuiccOtpk); n == 65 && s.BPPEuiccOtpk[0] != 0x04 {
		return fmt.Errorf("signing: bppEuiccOtpk uncompressed-point first byte 0x%02x (expected 0x04)", s.BPPEuiccOtpk[0])
	}
	return nil
}

// SignSmdpSigned2 builds the DER encoding, hashes it with SHA-256,
// and asks the broker to sign the digest with the SM-DP+'s DPpb
// (profile-binding) key. Returns (DER(SmdpSigned2), DER(ECDSA
// signature)).
//
// SGP.22 §H.5 mandates ECDSA-SHA-256 with the DPpb key. The DPpb
// key is distinct from DPauth (used for ServerSigned1) — both
// live in the same hsm-broker but have separate ceremony
// lifecycles. Operators rotate them on different cadences.
func SignSmdpSigned2(ctx context.Context, hc *hsmclient.Client, dppbKeyID string, payload SmdpSigned2) (signed, sig []byte, err error) {
	der, err := payload.MarshalDER()
	if err != nil {
		return nil, nil, err
	}
	digest := sha256.Sum256(der)
	resp, err := hc.Sign(ctx, dppbKeyID, digest[:])
	if err != nil {
		return nil, nil, fmt.Errorf("signing: HSM Sign (DPpb): %w", err)
	}
	return der, resp.SignatureDER, nil
}
