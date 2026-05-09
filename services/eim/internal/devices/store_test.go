package devices

import (
	"errors"
	"testing"

	eimv1 "github.com/ajamous/aether/services/eim/api/v1"
)

func TestMemory_RegisterAndList(t *testing.T) {
	s := NewMemoryStore()
	for _, e := range []string{"a", "b", "c"} {
		if err := s.RegisterDevice(&eimv1.Device{EID: eimv1.EID(e)}); err != nil {
			t.Fatalf("register %s: %v", e, err)
		}
	}
	if got := s.ListDevices(); len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
}

func TestMemory_RegisterRejectsDuplicate(t *testing.T) {
	s := NewMemoryStore()
	_ = s.RegisterDevice(&eimv1.Device{EID: "x"})
	if err := s.RegisterDevice(&eimv1.Device{EID: "x"}); !errors.Is(err, ErrDeviceExists) {
		t.Fatalf("expected ErrDeviceExists, got %v", err)
	}
}

func TestMemory_DeleteDeviceClearsItsCommands(t *testing.T) {
	s := NewMemoryStore()
	_ = s.RegisterDevice(&eimv1.Device{EID: "x"})
	if err := s.EnqueueCommand(&eimv1.Command{EID: "x", Kind: eimv1.CommandDownloadProfile}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := s.DeleteDevice("x"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := s.ListCommandsForDevice("x", true); len(got) != 0 {
		t.Fatalf("expected 0 commands after device delete, got %d", len(got))
	}
}

func TestMemory_EnqueueRejectsUnknownDevice(t *testing.T) {
	s := NewMemoryStore()
	if err := s.EnqueueCommand(&eimv1.Command{EID: "no-such", Kind: eimv1.CommandDownloadProfile}); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("expected ErrDeviceNotFound, got %v", err)
	}
}

func TestMemory_PollAndAckLifecycle(t *testing.T) {
	s := NewMemoryStore()
	_ = s.RegisterDevice(&eimv1.Device{EID: "x"})
	cmd := &eimv1.Command{EID: "x", Kind: eimv1.CommandDownloadProfile, SMDPAddress: "smdp.example"}
	if err := s.EnqueueCommand(cmd); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// IPA poll: only Pending commands should appear.
	pending := s.ListCommandsForDevice("x", false)
	if len(pending) != 1 || pending[0].State != eimv1.CommandStatePending {
		t.Fatalf("unexpected pending: %+v", pending)
	}

	// Mark delivered.
	if err := s.MarkDelivered(cmd.ID); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	got, _ := s.GetCommand(cmd.ID)
	if got.State != eimv1.CommandStateDelivered || got.DeliveredAt == nil {
		t.Fatalf("expected Delivered with timestamp: %+v", got)
	}

	// Ack with success.
	if err := s.AckCommand(cmd.ID, &eimv1.AckCommandRequest{State: eimv1.CommandStateCompleted}); err != nil {
		t.Fatalf("ack: %v", err)
	}
	got, _ = s.GetCommand(cmd.ID)
	if got.State != eimv1.CommandStateCompleted || got.CompletedAt == nil {
		t.Fatalf("expected Completed: %+v", got)
	}

	// Subsequent IPA poll without includeCompleted shouldn't see it.
	pending = s.ListCommandsForDevice("x", false)
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending, got %d", len(pending))
	}
}

func TestMemory_AckRejectsInvalidState(t *testing.T) {
	s := NewMemoryStore()
	_ = s.RegisterDevice(&eimv1.Device{EID: "x"})
	cmd := &eimv1.Command{EID: "x", Kind: eimv1.CommandEnableProfile}
	_ = s.EnqueueCommand(cmd)
	if err := s.AckCommand(cmd.ID, &eimv1.AckCommandRequest{State: eimv1.CommandStatePending}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestMemory_MarkDeliveredIsIdempotent(t *testing.T) {
	s := NewMemoryStore()
	_ = s.RegisterDevice(&eimv1.Device{EID: "x"})
	cmd := &eimv1.Command{EID: "x", Kind: eimv1.CommandDownloadProfile}
	_ = s.EnqueueCommand(cmd)
	for i := 0; i < 3; i++ {
		if err := s.MarkDelivered(cmd.ID); err != nil {
			t.Fatalf("deliver %d: %v", i, err)
		}
	}
}
