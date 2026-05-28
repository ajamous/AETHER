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
