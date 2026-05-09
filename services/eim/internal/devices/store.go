// Package devices holds the eIM's device + command-queue storage.
//
// One backend interface — Store — covers both. In-memory and
// Postgres implementations both satisfy it; the HTTP server
// accepts either.
package devices

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"

	eimv1 "github.com/ajamous/aether/services/eim/api/v1"
)

// Store is the contract for eIM persistence.
type Store interface {
	RegisterDevice(*eimv1.Device) error
	GetDevice(eimv1.EID) (*eimv1.Device, error)
	ListDevices() []*eimv1.Device
	DeleteDevice(eimv1.EID) error

	EnqueueCommand(*eimv1.Command) error
	GetCommand(commandID string) (*eimv1.Command, error)
	ListCommandsForDevice(eimv1.EID, bool /* includeCompleted */) []*eimv1.Command

	// MarkDelivered transitions a Pending command to Delivered, used
	// when an IPA poll picks it up.
	MarkDelivered(commandID string) error
	// AckCommand transitions a Delivered command to Completed or
	// Failed based on req.State.
	AckCommand(commandID string, req *eimv1.AckCommandRequest) error
}

var (
	ErrDeviceNotFound  = errors.New("eim: device not found")
	ErrDeviceExists    = errors.New("eim: device already registered")
	ErrCommandNotFound = errors.New("eim: command not found")
	ErrInvalidArgument = errors.New("eim: invalid argument")
)

// MemoryStore is the default in-memory store.
type MemoryStore struct {
	mu       sync.RWMutex
	devices  map[eimv1.EID]*eimv1.Device
	commands map[string]*eimv1.Command // keyed by command id
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		devices:  make(map[eimv1.EID]*eimv1.Device),
		commands: make(map[string]*eimv1.Command),
	}
}

// RegisterDevice inserts a new device. Returns ErrDeviceExists if the
// EID is already registered (idempotency is the operator's call —
// re-registering deliberately differs from updating).
func (s *MemoryStore) RegisterDevice(d *eimv1.Device) error {
	if d == nil || d.EID == "" {
		return ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[d.EID]; ok {
		return ErrDeviceExists
	}
	if d.RegisteredAt.IsZero() {
		d.RegisteredAt = time.Now().UTC()
	}
	s.devices[d.EID] = d
	return nil
}

func (s *MemoryStore) GetDevice(eid eimv1.EID) (*eimv1.Device, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[eid]
	if !ok {
		return nil, ErrDeviceNotFound
	}
	return d, nil
}

func (s *MemoryStore) ListDevices() []*eimv1.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*eimv1.Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RegisteredAt.Before(out[j].RegisteredAt) })
	return out
}

func (s *MemoryStore) DeleteDevice(eid eimv1.EID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[eid]; !ok {
		return ErrDeviceNotFound
	}
	delete(s.devices, eid)
	// Drop any pending commands for this device. A real production eIM
	// might prefer "tombstone" instead; for the lab, drop is honest.
	for id, c := range s.commands {
		if c.EID == eid {
			delete(s.commands, id)
		}
	}
	return nil
}

func (s *MemoryStore) EnqueueCommand(c *eimv1.Command) error {
	if c == nil || c.EID == "" || c.Kind == "" {
		return ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[c.EID]; !ok {
		return ErrDeviceNotFound
	}
	if c.ID == "" {
		c.ID = newCommandID()
	}
	if c.State == "" {
		c.State = eimv1.CommandStatePending
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	s.commands[c.ID] = c
	return nil
}

func (s *MemoryStore) GetCommand(id string) (*eimv1.Command, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.commands[id]
	if !ok {
		return nil, ErrCommandNotFound
	}
	return c, nil
}

func (s *MemoryStore) ListCommandsForDevice(eid eimv1.EID, includeCompleted bool) []*eimv1.Command {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*eimv1.Command, 0)
	for _, c := range s.commands {
		if c.EID != eid {
			continue
		}
		if !includeCompleted && (c.State == eimv1.CommandStateCompleted || c.State == eimv1.CommandStateFailed) {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (s *MemoryStore) MarkDelivered(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.commands[id]
	if !ok {
		return ErrCommandNotFound
	}
	if c.State != eimv1.CommandStatePending {
		return nil // idempotent — re-delivery is harmless
	}
	now := time.Now().UTC()
	c.State = eimv1.CommandStateDelivered
	c.DeliveredAt = &now
	return nil
}

func (s *MemoryStore) AckCommand(id string, req *eimv1.AckCommandRequest) error {
	if req == nil || (req.State != eimv1.CommandStateCompleted && req.State != eimv1.CommandStateFailed) {
		return ErrInvalidArgument
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.commands[id]
	if !ok {
		return ErrCommandNotFound
	}
	now := time.Now().UTC()
	c.State = req.State
	c.CompletedAt = &now
	c.FailureCode = req.FailureCode
	c.FailureNote = req.FailureNote
	if d, ok := s.devices[c.EID]; ok {
		d.LastSeen = &now
	}
	return nil
}

// newCommandID returns a 128-bit hex string suitable as an opaque
// command identifier.
func newCommandID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
