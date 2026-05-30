package profile

import (
	"bytes"
	"errors"
	"testing"
)

func TestMemoryStore_PutGet(t *testing.T) {
	s := NewMemoryStore()
	upp := []byte{0x30, 0x03, 0x01, 0x02, 0x03}
	if err := s.Put(&Prepared{ICCID: "8900000000000000001", UPP: upp}); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := s.Get("8900000000000000001")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(got.UPP, upp) {
		t.Errorf("UPP mismatch")
	}
}

func TestMemoryStore_GetMissing(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_PutValidation(t *testing.T) {
	s := NewMemoryStore()
	cases := []*Prepared{
		nil,
		{ICCID: "", UPP: []byte{1}},
		{ICCID: "x", UPP: nil},
	}
	for i, c := range cases {
		if err := s.Put(c); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestMemoryStore_Overwrite(t *testing.T) {
	s := NewMemoryStore()
	_ = s.Put(&Prepared{ICCID: "i", UPP: []byte{1}})
	_ = s.Put(&Prepared{ICCID: "i", UPP: []byte{2, 3}})
	got, _ := s.Get("i")
	if !bytes.Equal(got.UPP, []byte{2, 3}) {
		t.Errorf("overwrite failed: %x", got.UPP)
	}
}

func TestMemoryStore_GetByMatchingID(t *testing.T) {
	s := NewMemoryStore()
	upp := []byte{0x30, 0x03, 0x01, 0x02, 0x03}
	if err := s.Put(&Prepared{ICCID: "8900000000000000007", MatchingID: "abc123", UPP: upp}); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := s.GetByMatchingID("abc123")
	if err != nil {
		t.Fatalf("get-by-mid: %v", err)
	}
	if got.ICCID != "8900000000000000007" || !bytes.Equal(got.UPP, upp) {
		t.Errorf("got %+v", got)
	}
}

func TestMemoryStore_GetByMatchingID_Missing(t *testing.T) {
	s := NewMemoryStore()
	if _, err := s.GetByMatchingID(""); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty mid err = %v", err)
	}
	if _, err := s.GetByMatchingID("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown mid err = %v", err)
	}
}

// TestMemoryStore_MatchingIDReindexedOnReprepare confirms the
// matchingId secondary index follows the latest Put for the same
// ICCID: the old matchingId stops resolving, the new one resolves.
func TestMemoryStore_MatchingIDReindexedOnReprepare(t *testing.T) {
	s := NewMemoryStore()
	_ = s.Put(&Prepared{ICCID: "i", MatchingID: "old", UPP: []byte{1}})
	_ = s.Put(&Prepared{ICCID: "i", MatchingID: "new", UPP: []byte{2}})
	if _, err := s.GetByMatchingID("old"); !errors.Is(err, ErrNotFound) {
		t.Errorf("old matchingId should no longer resolve, got err = %v", err)
	}
	got, err := s.GetByMatchingID("new")
	if err != nil {
		t.Fatalf("new matchingId: %v", err)
	}
	if !bytes.Equal(got.UPP, []byte{2}) {
		t.Errorf("new resolves stale UPP: %x", got.UPP)
	}
}

func TestMemoryStore_PutWithoutMatchingID_StillIndexableByICCID(t *testing.T) {
	s := NewMemoryStore()
	if err := s.Put(&Prepared{ICCID: "i", UPP: []byte{1}}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := s.Get("i"); err != nil {
		t.Errorf("ICCID lookup should work without matchingId, got %v", err)
	}
	if _, err := s.GetByMatchingID(""); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty matchingId should not match anything, got %v", err)
	}
}
