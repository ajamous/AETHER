package session

import (
	"errors"
	"testing"
	"time"
)

func TestMemoryStore_CreateGetUpdateDelete(t *testing.T) {
	s := NewMemoryStore(0)
	tid := NewTransactionID()
	sess := &Session{
		TransactionID:  tid,
		State:          StateInitiated,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		EUICCChallenge: []byte("euicc-challenge-bytes"),
	}
	if err := s.Create(sess); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.Get(tid)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != StateInitiated {
		t.Fatalf("state = %q, want initiated", got.State)
	}

	got.State = StateAuthenticated
	if err := s.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := s.Get(tid)
	if got2.State != StateAuthenticated {
		t.Fatalf("state after update = %q", got2.State)
	}

	if err := s.Delete(tid); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = s.Get(tid)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_TTLExpiry(t *testing.T) {
	s := NewMemoryStore(50 * time.Millisecond)
	tid := NewTransactionID()
	sess := &Session{
		TransactionID: tid,
		UpdatedAt:     time.Now().Add(-1 * time.Second),
	}
	if err := s.Create(sess); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Get(tid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired session, got %v", err)
	}
}

func TestMemoryStore_RejectsEmptyTID(t *testing.T) {
	s := NewMemoryStore(0)
	if err := s.Create(&Session{}); err == nil {
		t.Fatal("expected error on empty transactionID")
	}
}
