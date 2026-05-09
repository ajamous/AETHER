// Package chain implements an append-only, hash-chained ledger for
// Aether's audit log.
//
// Each entry's hash binds:
//
//	hash_n = SHA-256(seq_n || ts_n || payload_n || hash_{n-1})
//
// hash_0 is computed against an all-zero prev_hash. Any tampering
// with any entry breaks the chain from that point forward; the
// Verify function detects it.
//
// Storage is pluggable. The default in-memory ledger is sufficient
// for the lab; a Postgres-backed ledger is the production target.
package chain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Entry is one record in the chain.
type Entry struct {
	Seq       uint64          `json:"seq"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
	PrevHash  []byte          `json:"prev_hash"`
	Hash      []byte          `json:"hash"`
}

// Ledger is the in-memory ledger.
type Ledger struct {
	mu      sync.RWMutex
	entries []*Entry
}

// NewLedger constructs an empty ledger.
func NewLedger() *Ledger { return &Ledger{} }

// Append adds an entry with the given payload and returns it.
func (l *Ledger) Append(payload json.RawMessage) (*Entry, error) {
	if len(payload) == 0 {
		return nil, errors.New("audit: empty payload")
	}
	if !json.Valid(payload) {
		return nil, errors.New("audit: payload is not valid JSON")
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	var prevHash []byte
	var seq uint64 = 1
	if n := len(l.entries); n > 0 {
		prevHash = l.entries[n-1].Hash
		seq = l.entries[n-1].Seq + 1
	} else {
		prevHash = make([]byte, sha256.Size)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	hash := computeHash(seq, now, payload, prevHash)
	e := &Entry{
		Seq:       seq,
		Timestamp: now,
		Payload:   append(json.RawMessage(nil), payload...),
		PrevHash:  prevHash,
		Hash:      hash,
	}
	l.entries = append(l.entries, e)
	return e, nil
}

// Get returns the entry at the given sequence number.
func (l *Ledger) Get(seq uint64) (*Entry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if seq == 0 || seq > uint64(len(l.entries)) {
		return nil, fmt.Errorf("audit: seq %d out of range", seq)
	}
	return l.entries[seq-1], nil
}

// List returns entries with seq > since (or all if since == 0).
func (l *Ledger) List(since uint64) []*Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]*Entry, 0, len(l.entries))
	for _, e := range l.entries {
		if e.Seq > since {
			out = append(out, e)
		}
	}
	return out
}

// Len returns the number of entries.
func (l *Ledger) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

// VerifyResult reports the chain's integrity.
type VerifyResult struct {
	OK          bool   `json:"ok"`
	Length      int    `json:"length"`
	FailedAtSeq uint64 `json:"failed_at_seq,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// Verify walks the chain and recomputes every hash.
func (l *Ledger) Verify() VerifyResult {
	l.mu.RLock()
	defer l.mu.RUnlock()

	prev := make([]byte, sha256.Size)
	for i, e := range l.entries {
		if uint64(i+1) != e.Seq {
			return VerifyResult{OK: false, Length: len(l.entries),
				FailedAtSeq: e.Seq, Reason: "sequence number out of order"}
		}
		if !bytesEqual(prev, e.PrevHash) {
			return VerifyResult{OK: false, Length: len(l.entries),
				FailedAtSeq: e.Seq, Reason: "prev_hash does not match previous entry's hash"}
		}
		want := computeHash(e.Seq, e.Timestamp, e.Payload, e.PrevHash)
		if !bytesEqual(want, e.Hash) {
			return VerifyResult{OK: false, Length: len(l.entries),
				FailedAtSeq: e.Seq, Reason: "hash does not match recomputed value"}
		}
		prev = e.Hash
	}
	return VerifyResult{OK: true, Length: len(l.entries)}
}

func computeHash(seq uint64, ts time.Time, payload []byte, prev []byte) []byte {
	h := sha256.New()
	var seqBuf [8]byte
	binary.BigEndian.PutUint64(seqBuf[:], seq)
	h.Write(seqBuf[:])
	tsBytes, _ := ts.UTC().MarshalBinary()
	h.Write(tsBytes)
	h.Write(payload)
	h.Write(prev)
	return h.Sum(nil)
}

func bytesEqual(a, b []byte) bool {
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
