// Postgres-backed implementation of the SM-DP+ session store.
//
// Sessions are short-lived (10 minutes by default per SGP.22 Annex A
// implementation guidance) so the table is small. A periodic cleanup
// of expired rows runs in the background.

package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGStore persists ES9+ sessions in PostgreSQL.
type PGStore struct {
	pool *pgxpool.Pool
	ttl  time.Duration

	cancelGC context.CancelFunc
}

const pgSchemaSQL = `
CREATE TABLE IF NOT EXISTS smdpp_sessions (
    transaction_id    TEXT PRIMARY KEY,
    state             TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL,
    updated_at        TIMESTAMPTZ NOT NULL,
    euicc_challenge   BYTEA,
    server_challenge  BYTEA,
    matching_id       TEXT NOT NULL DEFAULT '',
    iccid             TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS smdpp_sessions_updated_at_idx
  ON smdpp_sessions (updated_at);
`

// NewPGStore opens the pool, applies the schema, and returns a store.
// A background goroutine evicts sessions older than ttl every minute.
func NewPGStore(ctx context.Context, url string, ttl time.Duration) (*PGStore, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("smdpp/pg: connect: %w", err)
	}
	if _, err := pool.Exec(ctx, pgSchemaSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("smdpp/pg: migrate: %w", err)
	}
	gcCtx, cancel := context.WithCancel(context.Background())
	s := &PGStore{pool: pool, ttl: ttl, cancelGC: cancel}
	if ttl > 0 {
		go s.gcLoop(gcCtx)
	}
	return s, nil
}

// Close stops the GC goroutine and releases the pool.
func (s *PGStore) Close() {
	s.cancelGC()
	s.pool.Close()
}

func (s *PGStore) gcLoop(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cutoff := time.Now().UTC().Add(-s.ttl)
			_, _ = s.pool.Exec(ctx,
				`DELETE FROM smdpp_sessions WHERE updated_at < $1`, cutoff)
		}
	}
}

// Create inserts a session.
func (s *PGStore) Create(sess *Session) error {
	if sess.TransactionID == "" {
		return errors.New("session: empty transactionID")
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO smdpp_sessions
		   (transaction_id, state, created_at, updated_at,
		    euicc_challenge, server_challenge, matching_id, iccid)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		sess.TransactionID, string(sess.State),
		sess.CreatedAt, sess.UpdatedAt,
		sess.EUICCChallenge, sess.ServerChallenge,
		sess.MatchingID, sess.ICCID)
	if err != nil {
		return fmt.Errorf("smdpp/pg: create: %w", err)
	}
	return nil
}

// Get fetches a session by transactionID. Returns ErrNotFound if it
// doesn't exist or has expired beyond TTL.
func (s *PGStore) Get(tid string) (*Session, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT transaction_id, state, created_at, updated_at,
		        euicc_challenge, server_challenge, matching_id, iccid
		   FROM smdpp_sessions
		  WHERE transaction_id = $1`, tid)
	var (
		sess       Session
		state      string
		matchingID string
		iccid      string
	)
	if err := row.Scan(
		&sess.TransactionID, &state,
		&sess.CreatedAt, &sess.UpdatedAt,
		&sess.EUICCChallenge, &sess.ServerChallenge,
		&matchingID, &iccid,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("smdpp/pg: get: %w", err)
	}
	sess.State = State(state)
	sess.MatchingID = matchingID
	sess.ICCID = iccid
	if s.ttl > 0 && time.Since(sess.UpdatedAt) > s.ttl {
		_ = s.Delete(tid)
		return nil, ErrNotFound
	}
	return &sess, nil
}

// Update writes the session back, refreshing updated_at.
func (s *PGStore) Update(sess *Session) error {
	sess.UpdatedAt = time.Now().UTC()
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE smdpp_sessions
		    SET state = $2,
		        updated_at = $3,
		        euicc_challenge = $4,
		        server_challenge = $5,
		        matching_id = $6,
		        iccid = $7
		  WHERE transaction_id = $1`,
		sess.TransactionID, string(sess.State), sess.UpdatedAt,
		sess.EUICCChallenge, sess.ServerChallenge,
		sess.MatchingID, sess.ICCID)
	if err != nil {
		return fmt.Errorf("smdpp/pg: update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a session.
func (s *PGStore) Delete(tid string) error {
	_, err := s.pool.Exec(context.Background(),
		`DELETE FROM smdpp_sessions WHERE transaction_id = $1`, tid)
	if err != nil {
		return fmt.Errorf("smdpp/pg: delete: %w", err)
	}
	return nil
}
