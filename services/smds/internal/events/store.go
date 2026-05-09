// Package events holds the SM-DS event registry.
//
// An event is a pending notification: "there is a profile waiting at
// SMDP X for EID Y." The SM-DP+ creates an event via ES12; the LPA
// discovers it via ES11. Once the profile is delivered (or the order
// cancelled), the SM-DP+ deletes the event.
//
// Storage is pluggable. The default in-memory store is sufficient
// for the lab and for a single-instance deployment where the SM-DS
// can afford to lose pending events on restart. A Postgres-backed
// store lands when the rest of the persistence story does.
package events

import (
	"errors"
	"sort"
	"sync"
	"time"

	smdsv1 "github.com/ajamous/aether/services/smds/api/v1"
)

// Stored is one event held in the registry.
type Stored struct {
	EID          smdsv1.EID `json:"eid"`
	EventID      string     `json:"event_id"`
	RSPAddress   string     `json:"rsp_server_address"`
	Forwarding   bool       `json:"forwarding"`
	RegisteredAt time.Time  `json:"registered_at"`
}

// Store is the contract for event storage. Implementations must be
// safe for concurrent use.
type Store interface {
	Register(s *Stored) error
	Delete(eid smdsv1.EID, eventID string) error
	ListForEID(eid smdsv1.EID) []*Stored
	All() []*Stored
}

// ErrEventNotFound is returned when (eid, eventID) is not registered.
var ErrEventNotFound = errors.New("smds: event not found")

// MemoryStore is the in-memory store.
type MemoryStore struct {
	mu sync.RWMutex
	// keyed by (eid, eventID)
	events map[string]*Stored
}

// NewMemoryStore returns a fresh in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{events: make(map[string]*Stored)}
}

func key(eid smdsv1.EID, eventID string) string {
	return string(eid) + "|" + eventID
}

// Register stores or replaces the event identified by (EID, EventID).
// Idempotent: if the same key is registered again, the registration
// time updates and the rest is overwritten.
func (m *MemoryStore) Register(s *Stored) error {
	if s == nil || s.EID == "" || s.EventID == "" {
		return errors.New("smds: eid and event_id are required")
	}
	if s.RSPAddress == "" {
		return errors.New("smds: rsp_server_address is required")
	}
	if s.RegisteredAt.IsZero() {
		s.RegisteredAt = time.Now().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events[key(s.EID, s.EventID)] = s
	return nil
}

// Delete removes the event. Returns ErrEventNotFound if it didn't exist.
func (m *MemoryStore) Delete(eid smdsv1.EID, eventID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(eid, eventID)
	if _, ok := m.events[k]; !ok {
		return ErrEventNotFound
	}
	delete(m.events, k)
	return nil
}

// ListForEID returns events registered against the given EID, oldest
// first.
func (m *MemoryStore) ListForEID(eid smdsv1.EID) []*Stored {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Stored, 0)
	for _, e := range m.events {
		if e.EID == eid {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].RegisteredAt.Before(out[j].RegisteredAt)
	})
	return out
}

// All returns every registered event.
func (m *MemoryStore) All() []*Stored {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Stored, 0, len(m.events))
	for _, e := range m.events {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].RegisteredAt.Before(out[j].RegisteredAt)
	})
	return out
}
