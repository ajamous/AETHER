// Postgres-backed implementation of the audit ledger.
//
// Schema is created at startup if missing. The chain semantics are
// preserved exactly: each row binds the previous row's hash, the
// recompute logic is the same, and Verify walks rows in seq order.
//
// Append-only is enforced operationally — no UPDATE or DELETE
// statement exists in this package. A SAS-SM deployment should also
// REVOKE UPDATE/DELETE on the table from the application role; the
// schema migration includes a comment noting this.

package chain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PGLedger persists the audit chain in PostgreSQL.
type PGLedger struct {
	pool *pgxpool.Pool
}

const pgSchemaSQL = `
CREATE TABLE IF NOT EXISTS audit_entries (
    seq        BIGSERIAL PRIMARY KEY,
    ts         TIMESTAMPTZ NOT NULL,
    -- payload is BYTEA, not JSONB. The hash chain is over exact bytes;
    -- JSONB normalises whitespace and key ordering, which breaks the
    -- hash on round-trip. The application validates JSON before INSERT.
    payload    BYTEA NOT NULL,
    prev_hash  BYTEA NOT NULL,
    hash       BYTEA NOT NULL UNIQUE
);

-- Operationally append-only. SAS-SM deployments should additionally:
--   REVOKE UPDATE, DELETE ON audit_entries FROM aether_app;
COMMENT ON TABLE audit_entries IS
  'Aether hash-chained audit log. Append-only by application contract; '
  'production deployments REVOKE UPDATE/DELETE from the app role.';
`

// NewPGLedger opens a pool against url, applies the schema, and returns
// a ledger ready for use.
func NewPGLedger(ctx context.Context, url string) (*PGLedger, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("audit/pg: connect: %w", err)
	}
	if _, err := pool.Exec(ctx, pgSchemaSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("audit/pg: migrate: %w", err)
	}
	return &PGLedger{pool: pool}, nil
}

// Close releases the pool.
func (l *PGLedger) Close() { l.pool.Close() }

// Append adds an entry, computing prev_hash and hash inside a serializable
// transaction so concurrent appends produce a well-ordered chain.
func (l *PGLedger) Append(payload json.RawMessage) (*Entry, error) {
	if len(payload) == 0 {
		return nil, errors.New("audit: empty payload")
	}
	if !json.Valid(payload) {
		return nil, errors.New("audit: payload is not valid JSON")
	}
	ctx := context.Background()
	for {
		entry, retry, err := l.appendOnce(ctx, payload)
		if err != nil {
			return nil, err
		}
		if !retry {
			return entry, nil
		}
		// Serialization conflict — back off and retry. We cap retries
		// implicitly by the request's overall timeout.
		time.Sleep(5 * time.Millisecond)
	}
}

func (l *PGLedger) appendOnce(ctx context.Context, payload json.RawMessage) (*Entry, bool, error) {
	tx, err := l.pool.BeginTx(ctx, pgxBeginSerializable)
	if err != nil {
		return nil, false, fmt.Errorf("audit/pg: begin: %w", err)
	}
	rolled := false
	defer func() {
		if !rolled {
			_ = tx.Rollback(ctx)
		}
	}()

	var prevHash []byte
	var seq int64 = 1
	row := tx.QueryRow(ctx, `SELECT seq, hash FROM audit_entries ORDER BY seq DESC LIMIT 1`)
	var prevSeq int64
	if err := row.Scan(&prevSeq, &prevHash); err != nil {
		// pgx returns ErrNoRows when the table is empty; that's fine.
		if !isNoRows(err) {
			return nil, false, fmt.Errorf("audit/pg: read tail: %w", err)
		}
		prevHash = make([]byte, sha256.Size)
	} else {
		seq = prevSeq + 1
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	hash := computeHash(uint64(seq), now, payload, prevHash) //nolint:gosec // seq is monotonic-positive, fits in uint64

	if _, err := tx.Exec(ctx,
		`INSERT INTO audit_entries (seq, ts, payload, prev_hash, hash)
		 VALUES ($1, $2, $3, $4, $5)`,
		seq, now, []byte(payload), prevHash, hash); err != nil {
		if isSerializationFailure(err) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("audit/pg: insert: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		rolled = true
		if isSerializationFailure(err) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("audit/pg: commit: %w", err)
	}
	rolled = true
	return &Entry{
		Seq:       uint64(seq), //nolint:gosec // seq is monotonic-positive
		Timestamp: now,
		Payload:   append(json.RawMessage(nil), payload...),
		PrevHash:  prevHash,
		Hash:      hash,
	}, false, nil
}

// Get fetches a single entry by seq.
func (l *PGLedger) Get(seq uint64) (*Entry, error) {
	ctx := context.Background()
	row := l.pool.QueryRow(ctx,
		`SELECT seq, ts, payload, prev_hash, hash
		   FROM audit_entries WHERE seq = $1`, int64(seq)) //nolint:gosec // audit seq will not reach 2^63 in any realistic operator timeline
	return scanEntry(row)
}

// List returns entries with seq > since (or all if since == 0).
func (l *PGLedger) List(since uint64) []*Entry {
	ctx := context.Background()
	rows, err := l.pool.Query(ctx,
		`SELECT seq, ts, payload, prev_hash, hash
		   FROM audit_entries WHERE seq > $1 ORDER BY seq ASC`,
		int64(since)) //nolint:gosec // audit seq will not reach 2^63 in any realistic operator timeline
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			continue
		}
		out = append(out, e)
	}
	return out
}

// Len returns the row count.
func (l *PGLedger) Len() int {
	ctx := context.Background()
	var n int64
	if err := l.pool.QueryRow(ctx, `SELECT count(*) FROM audit_entries`).Scan(&n); err != nil {
		return 0
	}
	return int(n)
}

// Verify walks the chain in seq order and recomputes every hash.
func (l *PGLedger) Verify() VerifyResult {
	ctx := context.Background()
	rows, err := l.pool.Query(ctx,
		`SELECT seq, ts, payload, prev_hash, hash
		   FROM audit_entries ORDER BY seq ASC`)
	if err != nil {
		return VerifyResult{Reason: err.Error()}
	}
	defer rows.Close()

	expected := make([]byte, sha256.Size)
	count := 0
	var lastSeq uint64
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return VerifyResult{Length: count, Reason: err.Error()}
		}
		count++
		if e.Seq != lastSeq+1 {
			return VerifyResult{Length: count, FailedAtSeq: e.Seq,
				Reason: "sequence number out of order"}
		}
		lastSeq = e.Seq
		if !bytesEqual(expected, e.PrevHash) {
			return VerifyResult{Length: count, FailedAtSeq: e.Seq,
				Reason: "prev_hash does not match previous entry's hash"}
		}
		want := computeHash(e.Seq, e.Timestamp, e.Payload, e.PrevHash)
		if !bytesEqual(want, e.Hash) {
			return VerifyResult{Length: count, FailedAtSeq: e.Seq,
				Reason: "hash does not match recomputed value"}
		}
		expected = e.Hash
	}
	return VerifyResult{OK: true, Length: count}
}
