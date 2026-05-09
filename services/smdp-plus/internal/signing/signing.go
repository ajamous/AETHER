// Package signing produces the ES9+ signed payloads SGP.22 requires.
//
// Today this covers ServerSigned1 (SGP.22 §5.7.13) — the
// authentication preamble the SM-DP+ returns from
// ES9+/initiateAuthentication. The signature is ECDSA-SHA-256 (per
// §H.5) over the DER encoding of the payload, computed by the HSM
// broker so the private key never leaves the HSM.
//
// Other signed payloads (smdpSigned2 in §5.7.14, smdpSigned3 in
// §5.7.16, etc.) follow the same pattern; they will land here as the
// surrounding flows do.
package signing

import (
	"context"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ajamous/aether/pkg/hsmclient"
)

// ServerSigned1 mirrors SGP.22 §5.7.13:
//
//	ServerSigned1 ::= SEQUENCE {
//	    transactionId  TransactionId,        -- OCTET STRING (SIZE(1..16))
//	    euiccChallenge Octet16,              -- OCTET STRING (SIZE(16))
//	    serverAddress  UTF8String,
//	    serverChallenge Octet16              -- OCTET STRING (SIZE(16))
//	}
//
// Encoding follows DER. The struct field tags use Go's encoding/asn1
// defaults; the four field types match the spec ordering and types.
type ServerSigned1 struct {
	TransactionID   []byte
	EUICCChallenge  []byte
	ServerAddress   string `asn1:"utf8"`
	ServerChallenge []byte
}

// MarshalDER returns the DER encoding of the ServerSigned1 SEQUENCE.
func (s ServerSigned1) MarshalDER() ([]byte, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	return asn1.Marshal(s)
}

// UnmarshalServerSigned1 decodes a DER-encoded ServerSigned1.
func UnmarshalServerSigned1(b []byte) (*ServerSigned1, error) {
	var out ServerSigned1
	rest, err := asn1.Unmarshal(b, &out)
	if err != nil {
		return nil, fmt.Errorf("signing: ServerSigned1 unmarshal: %w", err)
	}
	if len(rest) != 0 {
		return nil, errors.New("signing: trailing bytes after ServerSigned1")
	}
	return &out, nil
}

func (s ServerSigned1) validate() error {
	if l := len(s.TransactionID); l < 1 || l > 16 {
		return fmt.Errorf("signing: transactionId length %d outside spec range 1..16", l)
	}
	if len(s.EUICCChallenge) != 16 {
		return fmt.Errorf("signing: euiccChallenge must be 16 bytes, got %d", len(s.EUICCChallenge))
	}
	if len(s.ServerChallenge) != 16 {
		return fmt.Errorf("signing: serverChallenge must be 16 bytes, got %d", len(s.ServerChallenge))
	}
	if s.ServerAddress == "" {
		return errors.New("signing: serverAddress required")
	}
	return nil
}

// SignServerSigned1 builds the DER encoding, hashes it with SHA-256,
// and asks the broker to sign the digest with the DPauth key.
// Returns (DER(ServerSigned1), DER(ECDSA signature)).
//
// SGP.22 §H.5 mandates ECDSA-SHA-256 with the DPauth key.
func SignServerSigned1(ctx context.Context, hc *hsmclient.Client, dpauthKeyID string, payload ServerSigned1) (signed, sig []byte, err error) {
	der, err := payload.MarshalDER()
	if err != nil {
		return nil, nil, err
	}
	digest := sha256.Sum256(der)
	resp, err := hc.Sign(ctx, dpauthKeyID, digest[:])
	if err != nil {
		return nil, nil, fmt.Errorf("signing: HSM Sign: %w", err)
	}
	return der, resp.SignatureDER, nil
}

// HexTransactionID returns the lowercase hex string form of a binary
// transactionId — the form the SM-DP+ shows in logs and the form the
// LPA echoes back on subsequent messages.
func HexTransactionID(tid []byte) string { return hex.EncodeToString(tid) }
