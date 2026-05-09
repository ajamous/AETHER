// Command aether-verify-anchor verifies a signed timeline anchor
// produced by the Aether audit service's /v1/anchor endpoint.
//
// Auditors run this against the daily anchor pulled from the
// immutable offsite bucket. The procedure is documented in
// docs/sas-sm/audit-retention.md.
//
// Usage:
//
//	aether-verify-anchor \
//	    --pubkey audit-anchor-pub.pem \
//	    --anchor anchor-2026-05-09.json
//
// Optional cross-checks against a recovered Postgres tail:
//
//	aether-verify-anchor \
//	    --pubkey audit-anchor-pub.pem \
//	    --anchor anchor-2026-05-09.json \
//	    --against-length 1234567 \
//	    --against-tail-hash abcdef0123...
//
// Exit status:
//
//	0  signature verifies; cross-checks (if any) match
//	1  bad input (file not found, malformed PEM/JSON, etc.)
//	2  signature does not verify
//	3  cross-check mismatch
//
// The tool is stdlib-only on purpose: an auditor's verifier must
// be reproducible from a single Go file with no third-party
// dependencies. The same code can be re-implemented in Python or
// any other language straight from the SGP.22 §H.5 + asn1.Marshal
// layout below.
package main

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
	"time"
)

// anchorJSON mirrors the response shape from `services/audit
// /v1/anchor` when signing is enabled.
type anchorJSON struct {
	Length        int64  `json:"length"`
	TailHash      string `json:"tail_hash"`
	Timestamp     string `json:"timestamp"`
	SignedPayload []byte `json:"signed_payload"`
	Signature     []byte `json:"signature"`
	SignatureAlg  string `json:"signature_alg"`
}

// anchorDER mirrors services/audit/internal/anchor.Anchor — the
// DER-encoded SEQUENCE the audit service signs over. Re-defined
// here (rather than imported) so this CLI stays a single
// stdlib-only file an auditor can audit themselves.
type anchorDER struct {
	Timestamp time.Time `asn1:"generalized"`
	Length    int64
	TailHash  []byte
}

const (
	exitOK         = 0
	exitBadInput   = 1
	exitBadSig     = 2
	exitBadReplay  = 3
)

func main() {
	os.Exit(run())
}

// flagCommandLineReset rebuilds flag.CommandLine so tests can
// invoke run() multiple times in one process. Production binaries
// don't call this.
func flagCommandLineReset() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
}

func run() int {
	var (
		pubkeyPath      = flag.String("pubkey", "", "PEM-encoded ECDSA P-256 public key (the audit-anchor-key as published in the SAS-SM evidence pack)")
		anchorPath      = flag.String("anchor", "", "Path to the JSON anchor file fetched from /v1/anchor (use - for stdin)")
		againstLength   = flag.Int64("against-length", -1, "Optional: cross-check the anchor's `length` against this value (typically a fresh Postgres restore's row count)")
		againstTailHash = flag.String("against-tail-hash", "", "Optional: cross-check the anchor's `tail_hash` against this lowercase-hex string")
	)
	flag.Parse()

	if *pubkeyPath == "" || *anchorPath == "" {
		fmt.Fprintln(os.Stderr, "usage: aether-verify-anchor --pubkey FILE --anchor FILE [--against-length N --against-tail-hash HEX]")
		return exitBadInput
	}

	pub, err := loadPublicKey(*pubkeyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load pubkey: %v\n", err)
		return exitBadInput
	}

	anchor, err := loadAnchor(*anchorPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load anchor: %v\n", err)
		return exitBadInput
	}

	if anchor.SignatureAlg != "ECDSA-SHA-256" {
		fmt.Fprintf(os.Stderr, "unsupported signature_alg %q (want ECDSA-SHA-256)\n", anchor.SignatureAlg)
		return exitBadInput
	}
	if len(anchor.SignedPayload) == 0 {
		fmt.Fprintln(os.Stderr, "anchor has no signed_payload — was the audit service started without --hsm-broker?")
		return exitBadInput
	}
	if len(anchor.Signature) == 0 {
		fmt.Fprintln(os.Stderr, "anchor has no signature")
		return exitBadInput
	}

	// Decode the DER signed payload and cross-check that its fields
	// agree with the JSON shape the operator pulled from /v1/anchor.
	// A discrepancy means the bucket was tampered with after the
	// audit service signed the payload.
	var inner anchorDER
	if rest, err := asn1.Unmarshal(anchor.SignedPayload, &inner); err != nil {
		fmt.Fprintf(os.Stderr, "decode signed_payload: %v\n", err)
		return exitBadInput
	} else if len(rest) != 0 {
		fmt.Fprintln(os.Stderr, "decode signed_payload: trailing bytes")
		return exitBadInput
	}
	if inner.Length != anchor.Length {
		fmt.Fprintf(os.Stderr, "DER length %d does not match JSON length %d — anchor file is inconsistent\n", inner.Length, anchor.Length)
		return exitBadSig
	}
	if hex.EncodeToString(inner.TailHash) != anchor.TailHash {
		fmt.Fprintln(os.Stderr, "DER tail_hash does not match JSON tail_hash — anchor file is inconsistent")
		return exitBadSig
	}

	// SHA-256 the signed_payload and ECDSA-Verify against the
	// published audit-anchor public key.
	digest := sha256.Sum256(anchor.SignedPayload)
	var sig struct{ R, S *big.Int }
	if rest, err := asn1.Unmarshal(anchor.Signature, &sig); err != nil {
		fmt.Fprintf(os.Stderr, "decode signature: %v\n", err)
		return exitBadInput
	} else if len(rest) != 0 {
		fmt.Fprintln(os.Stderr, "decode signature: trailing bytes")
		return exitBadInput
	}
	if !ecdsa.Verify(pub, digest[:], sig.R, sig.S) {
		fmt.Fprintln(os.Stderr, "ECDSA verify FAILED — signature does not match the supplied public key")
		return exitBadSig
	}

	fmt.Printf("signature OK: length=%d tail_hash=%s timestamp=%s\n", anchor.Length, anchor.TailHash, anchor.Timestamp)

	// Optional replay checks. These let an auditor confirm that
	// the chain they restored from backup matches the day's
	// anchor — the audit service signed for THIS chain state, not
	// a forgery.
	if *againstLength >= 0 && *againstLength != anchor.Length {
		fmt.Fprintf(os.Stderr, "REPLAY MISMATCH: anchor length=%d, you supplied length=%d\n", anchor.Length, *againstLength)
		return exitBadReplay
	}
	if *againstTailHash != "" {
		got := strings.ToLower(strings.TrimSpace(*againstTailHash))
		if got != anchor.TailHash {
			fmt.Fprintf(os.Stderr, "REPLAY MISMATCH: anchor tail_hash=%s, you supplied tail_hash=%s\n", anchor.TailHash, got)
			return exitBadReplay
		}
	}
	if *againstLength >= 0 || *againstTailHash != "" {
		fmt.Println("replay OK: chain state matches anchor")
	}

	return exitOK
}

func loadPublicKey(path string) (*ecdsa.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("not a PEM-encoded file")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// Some operators publish the SubjectPublicKeyInfo in
		// CERTIFICATE form. Try that too.
		if cert, cerr := x509.ParseCertificate(block.Bytes); cerr == nil {
			pub = cert.PublicKey
		} else {
			return nil, fmt.Errorf("parse pubkey: %w", err)
		}
	}
	ec, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("pubkey is %T, want *ecdsa.PublicKey", pub)
	}
	return ec, nil
}

func loadAnchor(path string) (*anchorJSON, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	var a anchorJSON
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("parse anchor JSON: %w", err)
	}
	return &a, nil
}
