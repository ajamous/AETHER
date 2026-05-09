package chain

import (
	"encoding/json"
	"testing"
)

func TestLedger_AppendAndVerify(t *testing.T) {
	l := NewLedger()
	for i := 0; i < 10; i++ {
		_, err := l.Append(json.RawMessage(`{"event":"test"}`))
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if l.Len() != 10 {
		t.Fatalf("len = %d", l.Len())
	}
	r := l.Verify()
	if !r.OK {
		t.Fatalf("verify failed: %+v", r)
	}
}

func TestLedger_TamperDetected(t *testing.T) {
	l := NewLedger()
	for i := 0; i < 5; i++ {
		_, _ = l.Append(json.RawMessage(`{"event":"test"}`))
	}
	// Tamper with entry 3.
	l.entries[2].Payload = json.RawMessage(`{"event":"tampered"}`)
	r := l.Verify()
	if r.OK {
		t.Fatal("expected verify to fail after tampering")
	}
	if r.FailedAtSeq != 3 {
		t.Fatalf("FailedAtSeq = %d, want 3", r.FailedAtSeq)
	}
}

func TestLedger_AppendRejectsInvalidJSON(t *testing.T) {
	l := NewLedger()
	if _, err := l.Append(json.RawMessage("{not json")); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
	if _, err := l.Append(json.RawMessage("")); err == nil {
		t.Fatal("expected error on empty payload")
	}
}

func TestLedger_GetByseq(t *testing.T) {
	l := NewLedger()
	for i := 0; i < 3; i++ {
		_, _ = l.Append(json.RawMessage(`{}`))
	}
	e, err := l.Get(2)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if e.Seq != 2 {
		t.Fatalf("seq = %d", e.Seq)
	}
	if _, err := l.Get(0); err == nil {
		t.Fatal("expected error on seq=0")
	}
	if _, err := l.Get(99); err == nil {
		t.Fatal("expected error on out-of-range seq")
	}
}

func TestLedger_ListSince(t *testing.T) {
	l := NewLedger()
	for i := 0; i < 5; i++ {
		_, _ = l.Append(json.RawMessage(`{}`))
	}
	got := l.List(3)
	if len(got) != 2 {
		t.Fatalf("len(since=3) = %d, want 2", len(got))
	}
	if got[0].Seq != 4 || got[1].Seq != 5 {
		t.Fatalf("unexpected seqs: %+v", got)
	}
}
