package chain

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"
)

// Postgres integration tests. Skipped unless AETHER_PG_URL points at
// a reachable database. CI brings up a Postgres service container and
// sets the env; local runs need:
//
//	export AETHER_PG_URL='postgres://aether:aether@127.0.0.1:5432/aether_test?sslmode=disable'

func openOrSkip(t *testing.T) *PGLedger {
	t.Helper()
	url := os.Getenv("AETHER_PG_URL")
	if url == "" {
		t.Skip("AETHER_PG_URL not set; skipping Postgres integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	l, err := NewPGLedger(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	// Each test starts from a clean table so they don't see each other's
	// rows. We can TRUNCATE because we own the schema in the test DB.
	if _, err := l.pool.Exec(ctx, `TRUNCATE TABLE audit_entries RESTART IDENTITY`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return l
}

func TestPGLedger_AppendAndVerify(t *testing.T) {
	l := openOrSkip(t)
	for i := 0; i < 20; i++ {
		if _, err := l.Append(json.RawMessage(`{"event":"login"}`)); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	if l.Len() != 20 {
		t.Fatalf("len = %d", l.Len())
	}
	r := l.Verify()
	if !r.OK {
		t.Fatalf("verify failed: %+v", r)
	}
}

func TestPGLedger_TamperDetected(t *testing.T) {
	l := openOrSkip(t)
	for i := 0; i < 5; i++ {
		_, _ = l.Append(json.RawMessage(`{"event":"x"}`))
	}
	// Sneak past the application's append-only contract by writing
	// SQL directly. A real SAS-SM deployment would have GRANTs that
	// prevent this; here we simulate the auditor scenario.
	_, err := l.pool.Exec(context.Background(),
		`UPDATE audit_entries SET payload = '{"event":"tampered"}' WHERE seq = 3`)
	if err != nil {
		t.Fatalf("simulate tamper: %v", err)
	}
	r := l.Verify()
	if r.OK {
		t.Fatal("expected verify to fail after tamper")
	}
	if r.FailedAtSeq != 3 {
		t.Fatalf("FailedAtSeq = %d, want 3", r.FailedAtSeq)
	}
}

func TestPGLedger_GetByseq(t *testing.T) {
	l := openOrSkip(t)
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
	if _, err := l.Get(99); err == nil {
		t.Fatal("expected error on out-of-range seq")
	}
}

func TestPGLedger_ListSince(t *testing.T) {
	l := openOrSkip(t)
	for i := 0; i < 5; i++ {
		_, _ = l.Append(json.RawMessage(`{}`))
	}
	got := l.List(3)
	if len(got) != 2 || got[0].Seq != 4 || got[1].Seq != 5 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestPGLedger_AppendRejectsInvalidJSON(t *testing.T) {
	l := openOrSkip(t)
	if _, err := l.Append(json.RawMessage("{not json")); err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestPGLedger_ConcurrentAppendsKeepChainIntact(t *testing.T) {
	l := openOrSkip(t)
	const writers = 8
	const perWriter = 25
	var wg sync.WaitGroup
	wg.Add(writers)
	for w := 0; w < writers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				if _, err := l.Append(json.RawMessage(`{"x":1}`)); err != nil {
					t.Errorf("append: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if l.Len() != writers*perWriter {
		t.Fatalf("len = %d, want %d", l.Len(), writers*perWriter)
	}
	r := l.Verify()
	if !r.OK {
		t.Fatalf("chain corrupted under concurrent writers: %+v", r)
	}
}
