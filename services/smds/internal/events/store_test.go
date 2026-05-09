package events

import (
	"errors"
	"testing"

	smdsv1 "github.com/ajamous/aether/services/smds/api/v1"
)

func TestMemoryStore_RegisterAndList(t *testing.T) {
	s := NewMemoryStore()
	eid := smdsv1.EID("89049032123451234512345678901234")

	if err := s.Register(&Stored{EID: eid, EventID: "e1", RSPAddress: "smdp.example"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := s.Register(&Stored{EID: eid, EventID: "e2", RSPAddress: "smdp.example"}); err != nil {
		t.Fatalf("register e2: %v", err)
	}
	out := s.ListForEID(eid)
	if len(out) != 2 {
		t.Fatalf("expected 2 events, got %d", len(out))
	}
	other := smdsv1.EID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if got := s.ListForEID(other); len(got) != 0 {
		t.Fatalf("expected 0 events for other EID, got %d", len(got))
	}
}

func TestMemoryStore_Idempotent(t *testing.T) {
	s := NewMemoryStore()
	eid := smdsv1.EID("89049032123451234512345678901234")
	for i := 0; i < 3; i++ {
		if err := s.Register(&Stored{EID: eid, EventID: "e1", RSPAddress: "smdp.example"}); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
	}
	if got := len(s.ListForEID(eid)); got != 1 {
		t.Fatalf("expected 1 unique event, got %d", got)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	s := NewMemoryStore()
	eid := smdsv1.EID("89049032123451234512345678901234")
	_ = s.Register(&Stored{EID: eid, EventID: "e1", RSPAddress: "smdp.example"})

	if err := s.Delete(eid, "e1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := len(s.ListForEID(eid)); got != 0 {
		t.Fatalf("expected 0 after delete, got %d", got)
	}
	if err := s.Delete(eid, "e1"); !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound on second delete, got %v", err)
	}
}

func TestMemoryStore_RejectsMissingFields(t *testing.T) {
	s := NewMemoryStore()
	if err := s.Register(&Stored{}); err == nil {
		t.Fatal("expected error on empty register")
	}
	if err := s.Register(&Stored{EID: "x", EventID: "y"}); err == nil {
		t.Fatal("expected error on missing rsp_server_address")
	}
}
