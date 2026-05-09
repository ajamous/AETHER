package events

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	smdsv1 "github.com/ajamous/aether/services/smds/api/v1"
)

func openOrSkip(t *testing.T) *PGStore {
	t.Helper()
	url := os.Getenv("AETHER_PG_URL")
	if url == "" {
		t.Skip("AETHER_PG_URL not set; skipping Postgres integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := NewPGStore(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.pool.Exec(ctx, `TRUNCATE TABLE smds_events`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return s
}

func TestPGStore_RegisterAndList(t *testing.T) {
	s := openOrSkip(t)
	eid := smdsv1.EID("89049032123451234512345678901234")

	if err := s.Register(&Stored{EID: eid, EventID: "e1", RSPAddress: "smdp.example"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := s.Register(&Stored{EID: eid, EventID: "e2", RSPAddress: "smdp.example"}); err != nil {
		t.Fatalf("register e2: %v", err)
	}
	out := s.ListForEID(eid)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
}

func TestPGStore_Idempotent(t *testing.T) {
	s := openOrSkip(t)
	eid := smdsv1.EID("aa49032123451234512345678901234a")
	for i := 0; i < 3; i++ {
		if err := s.Register(&Stored{EID: eid, EventID: "e1", RSPAddress: "smdp.example"}); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
	}
	if got := len(s.ListForEID(eid)); got != 1 {
		t.Fatalf("expected 1 unique, got %d", got)
	}
}

func TestPGStore_Delete(t *testing.T) {
	s := openOrSkip(t)
	eid := smdsv1.EID("bb49032123451234512345678901234b")
	_ = s.Register(&Stored{EID: eid, EventID: "e1", RSPAddress: "smdp.example"})

	if err := s.Delete(eid, "e1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := len(s.ListForEID(eid)); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if err := s.Delete(eid, "e1"); !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound on second delete, got %v", err)
	}
}

func TestPGStore_RejectsMissingFields(t *testing.T) {
	s := openOrSkip(t)
	if err := s.Register(&Stored{}); err == nil {
		t.Fatal("expected error on empty register")
	}
	if err := s.Register(&Stored{EID: "x", EventID: "y"}); err == nil {
		t.Fatal("expected error on missing rsp_server_address")
	}
}
