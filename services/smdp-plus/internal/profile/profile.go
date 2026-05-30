// Package profile holds prepared profiles: the DER-encoded SAIP UPP
// that profile-builder produced for a given ICCID, waiting for the
// eUICC to download it.
//
// In a full SGP.22 deployment the BSS creates these via ES2+
// DownloadOrder + ConfirmOrder before the LPA ever connects; the
// SM-DP+ then resolves the prepared profile from the matchingId the
// activation code carries. Until the gateway's ES2+ surface is live,
// smdp-plus's POST /v1/profiles/prepare is the in-tree stand-in, and
// the BPP request resolves the prepared profile by ICCID directly.
package profile

import (
	"errors"
	"sync"
)

// Prepared is a profile that has been built and is ready to seal into
// a BoundProfilePackage for the eUICC that downloads it.
type Prepared struct {
	// ICCID is the profile's identifier; the primary lookup key.
	ICCID string
	// MatchingID is the SGP.22 §4.1 activation-code token an LPA
	// presents at authenticateClient time (ctxParams1.matchingId).
	// The SM-DP+ uses it to resolve which prepared profile the eUICC
	// is downloading. Empty when the profile was prepared without one
	// (back-compat lab path).
	MatchingID string
	// UPP is the DER-encoded SAIP ProfilePackage profile-builder
	// produced (header + PE-USIM + PE-AKAParameter + PEEnd).
	UPP []byte
}

// Store is the contract for prepared-profile persistence.
// Implementations must be safe for concurrent use.
type Store interface {
	Put(p *Prepared) error
	Get(iccid string) (*Prepared, error)
	GetByMatchingID(matchingID string) (*Prepared, error)
}

// ErrNotFound is returned when an ICCID or matchingId has no prepared
// profile.
var ErrNotFound = errors.New("profile: not found")

// MemoryStore is the default in-memory prepared-profile store.
//
// It maintains a secondary index from matchingId to ICCID so
// GetByMatchingID is O(1). Both indices are updated under the same
// lock so a successful Put is fully visible to both lookups.
type MemoryStore struct {
	mu           sync.Mutex
	byICCID      map[string]*Prepared
	byMatchingID map[string]string // matchingId → iccid
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byICCID:      make(map[string]*Prepared),
		byMatchingID: make(map[string]string),
	}
}

func (s *MemoryStore) Put(p *Prepared) error {
	if p == nil {
		return errors.New("profile: nil Prepared")
	}
	if p.ICCID == "" {
		return errors.New("profile: empty ICCID")
	}
	if len(p.UPP) == 0 {
		return errors.New("profile: empty UPP")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Drop the previous matchingId index entry if this ICCID is being
	// re-prepared with a different matchingId (or no matchingId).
	if prev, ok := s.byICCID[p.ICCID]; ok && prev.MatchingID != "" && prev.MatchingID != p.MatchingID {
		delete(s.byMatchingID, prev.MatchingID)
	}
	s.byICCID[p.ICCID] = p
	if p.MatchingID != "" {
		s.byMatchingID[p.MatchingID] = p.ICCID
	}
	return nil
}

func (s *MemoryStore) Get(iccid string) (*Prepared, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byICCID[iccid]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (s *MemoryStore) GetByMatchingID(matchingID string) (*Prepared, error) {
	if matchingID == "" {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	iccid, ok := s.byMatchingID[matchingID]
	if !ok {
		return nil, ErrNotFound
	}
	p, ok := s.byICCID[iccid]
	if !ok {
		// Index drift — drop the stale matchingId entry.
		delete(s.byMatchingID, matchingID)
		return nil, ErrNotFound
	}
	return p, nil
}
