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
	// ICCID is the profile's identifier; the lookup key.
	ICCID string
	// UPP is the DER-encoded SAIP ProfilePackage profile-builder
	// produced (header + PE-USIM + PE-AKAParameter + PEEnd).
	UPP []byte
}

// Store is the contract for prepared-profile persistence.
// Implementations must be safe for concurrent use.
type Store interface {
	Put(p *Prepared) error
	Get(iccid string) (*Prepared, error)
}

// ErrNotFound is returned when an ICCID has no prepared profile.
var ErrNotFound = errors.New("profile: not found")

// MemoryStore is the default in-memory prepared-profile store.
type MemoryStore struct {
	mu  sync.Mutex
	bag map[string]*Prepared
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{bag: make(map[string]*Prepared)}
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
	s.bag[p.ICCID] = p
	return nil
}

func (s *MemoryStore) Get(iccid string) (*Prepared, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.bag[iccid]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}
