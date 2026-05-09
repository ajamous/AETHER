package devices

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	eimv1 "github.com/ajamous/aether/services/eim/api/v1"
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
	t.Cleanup(s.Close)
	if _, err := s.pool.Exec(ctx, `TRUNCATE TABLE eim_commands, eim_devices RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return s
}

func TestPG_RegisterListDelete(t *testing.T) {
	s := openOrSkip(t)
	for _, e := range []string{"a-eid", "b-eid"} {
		if err := s.RegisterDevice(&eimv1.Device{EID: eimv1.EID(e), Label: "lab"}); err != nil {
			t.Fatalf("register %s: %v", e, err)
		}
	}
	if got := s.ListDevices(); len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if err := s.DeleteDevice("a-eid"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetDevice("a-eid"); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("expected ErrDeviceNotFound, got %v", err)
	}
}

func TestPG_RegisterRejectsDuplicate(t *testing.T) {
	s := openOrSkip(t)
	_ = s.RegisterDevice(&eimv1.Device{EID: "x"})
	if err := s.RegisterDevice(&eimv1.Device{EID: "x"}); !errors.Is(err, ErrDeviceExists) {
		t.Fatalf("expected ErrDeviceExists, got %v", err)
	}
}

func TestPG_CommandLifecycle(t *testing.T) {
	s := openOrSkip(t)
	_ = s.RegisterDevice(&eimv1.Device{EID: "iot-1"})
	cmd := &eimv1.Command{
		EID: "iot-1", Kind: eimv1.CommandDownloadProfile,
		SMDPAddress: "smdp.example", MatchingID: "ABC",
	}
	if err := s.EnqueueCommand(cmd); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	pending := s.ListCommandsForDevice("iot-1", false)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}

	if err := s.MarkDelivered(cmd.ID); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	got, _ := s.GetCommand(cmd.ID)
	if got.State != eimv1.CommandStateDelivered {
		t.Fatalf("state = %q, want delivered", got.State)
	}

	if err := s.AckCommand(cmd.ID, &eimv1.AckCommandRequest{State: eimv1.CommandStateCompleted}); err != nil {
		t.Fatalf("ack: %v", err)
	}
	got, _ = s.GetCommand(cmd.ID)
	if got.State != eimv1.CommandStateCompleted {
		t.Fatalf("state = %q, want completed", got.State)
	}

	// Device's last_seen should now be set.
	dev, _ := s.GetDevice("iot-1")
	if dev.LastSeen == nil {
		t.Fatal("expected device last_seen to be set after ack")
	}
}

func TestPG_DeleteDeviceCascadesCommands(t *testing.T) {
	s := openOrSkip(t)
	_ = s.RegisterDevice(&eimv1.Device{EID: "x"})
	_ = s.EnqueueCommand(&eimv1.Command{EID: "x", Kind: eimv1.CommandEnableProfile})
	if err := s.DeleteDevice("x"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := s.ListCommandsForDevice("x", true); len(got) != 0 {
		t.Fatalf("expected commands cascade-deleted, got %d", len(got))
	}
}

func TestPG_EnqueueRejectsUnknownDevice(t *testing.T) {
	s := openOrSkip(t)
	if err := s.EnqueueCommand(&eimv1.Command{EID: "missing", Kind: eimv1.CommandEnableProfile}); !errors.Is(err, ErrDeviceNotFound) {
		t.Fatalf("expected ErrDeviceNotFound, got %v", err)
	}
}
