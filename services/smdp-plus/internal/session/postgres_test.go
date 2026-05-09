package session

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func openOrSkip(t *testing.T, ttl time.Duration) *PGStore {
	t.Helper()
	url := os.Getenv("AETHER_PG_URL")
	if url == "" {
		t.Skip("AETHER_PG_URL not set; skipping Postgres integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := NewPGStore(ctx, url, ttl)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(s.Close)
	if _, err := s.pool.Exec(ctx, `TRUNCATE TABLE smdpp_sessions`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return s
}

func TestPGSession_CreateGetUpdate(t *testing.T) {
	s := openOrSkip(t, 10*time.Minute)
	tid := NewTransactionID()
	now := time.Now().UTC()
	sess := &Session{
		TransactionID:  tid,
		State:          StateInitiated,
		CreatedAt:      now,
		UpdatedAt:      now,
		EUICCChallenge: []byte("euicc-challenge"),
	}
	if err := s.Create(sess); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.Get(tid)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != StateInitiated {
		t.Fatalf("state = %q", got.State)
	}
	if string(got.EUICCChallenge) != "euicc-challenge" {
		t.Fatalf("challenge mismatch: %s", got.EUICCChallenge)
	}

	got.State = StateAuthenticated
	if err := s.Update(got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := s.Get(tid)
	if got2.State != StateAuthenticated {
		t.Fatalf("after update state = %q", got2.State)
	}
}

func TestPGSession_DeleteThenGet(t *testing.T) {
	s := openOrSkip(t, 10*time.Minute)
	tid := NewTransactionID()
	now := time.Now().UTC()
	_ = s.Create(&Session{
		TransactionID: tid, State: StateInitiated,
		CreatedAt: now, UpdatedAt: now,
	})
	if err := s.Delete(tid); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(tid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPGSession_TTLExpiry(t *testing.T) {
	s := openOrSkip(t, 50*time.Millisecond)
	tid := NewTransactionID()
	old := time.Now().UTC().Add(-1 * time.Second)
	if err := s.Create(&Session{
		TransactionID: tid, State: StateInitiated,
		CreatedAt: old, UpdatedAt: old,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Get(tid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for expired session, got %v", err)
	}
}

func TestPGSession_UpdateMissing(t *testing.T) {
	s := openOrSkip(t, 10*time.Minute)
	if err := s.Update(&Session{TransactionID: "no-such-tid"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
