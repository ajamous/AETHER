// Package anchor produces signed timeline anchors for the audit
// chain.
//
// A timeline anchor is a periodic, externally-recorded snapshot of
// the chain state — `(length, tail_hash, timestamp)`. The audit
// retention runbook (docs/sas-sm/audit-retention.md) calls for one
// per day, written to the immutable offsite bucket. With this
// package, the anchor is also ECDSA-signed by the audit service
// (a key separate from the SM-DP+ identity hierarchy), so an
// auditor can verify offline that the anchor was issued by the
// audit service and not forged by an attacker who later compromised
// Postgres.
//
// The signing flow is a deliberate copy of the
// smdp-plus/smds ServerSigned1 pattern: DER encoding, SHA-256
// digest, ECDSA-P256 signature via hsm-broker. Keeping it
// per-service (rather than sharing a package) keeps the audit
// signing key's role + lifecycle independent of the SM-DP+'s
// identity keys.
package anchor

import (
	"context"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ajamous/aether/pkg/hsmclient"
)

// Anchor is the structure that gets DER-encoded and signed.
//
//	Anchor ::= SEQUENCE {
//	    timestamp  GeneralizedTime,
//	    length     INTEGER,
//	    tailHash   OCTET STRING (SIZE(32))     -- SHA-256
//	}
//
// The DER form is what the auditor stores and verifies; the
// JSON-friendly Response struct below is what `/v1/anchor`
// returns to the operator.
type Anchor struct {
	Timestamp time.Time `asn1:"generalized"`
	Length    int64
	TailHash  []byte
}

// MarshalDER returns the DER encoding of the anchor SEQUENCE.
func (a Anchor) MarshalDER() ([]byte, error) {
	if err := a.validate(); err != nil {
		return nil, err
	}
	return asn1.Marshal(a)
}

// UnmarshalAnchor decodes a DER-encoded anchor. Verifiers use this
// to round-trip the structure before checking the signature.
func UnmarshalAnchor(b []byte) (*Anchor, error) {
	var out Anchor
	rest, err := asn1.Unmarshal(b, &out)
	if err != nil {
		return nil, fmt.Errorf("anchor: unmarshal: %w", err)
	}
	if len(rest) != 0 {
		return nil, errors.New("anchor: trailing bytes")
	}
	return &out, nil
}

func (a Anchor) validate() error {
	if a.Length < 0 {
		return fmt.Errorf("anchor: negative length %d", a.Length)
	}
	if len(a.TailHash) != sha256.Size {
		return fmt.Errorf("anchor: tail hash must be %d bytes, got %d", sha256.Size, len(a.TailHash))
	}
	if a.Timestamp.IsZero() {
		return errors.New("anchor: timestamp required")
	}
	return nil
}

// Sign builds the DER encoding, hashes it with SHA-256, and asks
// the broker to sign the digest with the audit anchor key.
// Returns (DER(Anchor), DER(ECDSA signature)).
func Sign(ctx context.Context, hc *hsmclient.Client, keyID string, a Anchor) (signed, sig []byte, err error) {
	der, err := a.MarshalDER()
	if err != nil {
		return nil, nil, err
	}
	digest := sha256.Sum256(der)
	resp, err := hc.Sign(ctx, keyID, digest[:])
	if err != nil {
		return nil, nil, fmt.Errorf("anchor: HSM sign: %w", err)
	}
	return der, resp.SignatureDER, nil
}

// HexHash returns the lowercase-hex form of a 32-byte hash. Used
// in the JSON Response shape so operators piping through `jq` see
// the same form they'd cut-and-paste from logs.
func HexHash(h []byte) string { return hex.EncodeToString(h) }
