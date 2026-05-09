// Postgres-backed implementation of the SM-DS event store.
//
// Schema is created at startup if missing. Idempotent registration
// is enforced by the (eid, event_id) primary key + ON CONFLICT
// DO UPDATE clause that refreshes the rsp_address and registered_at.

package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	smdsv1 "github.com/ajamous/aether/services/smds/api/v1"
)

// PGStore persists SM-DS events in PostgreSQL.
type PGStore struct {
	pool *pgxpool.Pool
}

const pgSchemaSQL = `
CREATE TABLE IF NOT EXISTS smds_events (
    eid             TEXT NOT NULL,
    event_id        TEXT NOT NULL,
    rsp_address     TEXT NOT NULL,
    forwarding      BOOLEAN NOT NULL DEFAULT FALSE,
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (eid, event_id)
);

CREATE INDEX IF NOT EXISTS smds_events_eid_idx ON smds_events (eid);
`

// NewPGStore opens a pool, applies the schema, and returns a store.
func NewPGStore(ctx context.Context, url string) (*PGStore, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("smds/pg: connect: %w", err)
	}
	if _, err := pool.Exec(ctx, pgSchemaSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("smds/pg: migrate: %w", err)
	}
	return &PGStore{pool: pool}, nil
}

// Close releases the pool.
func (s *PGStore) Close() { s.pool.Close() }

// Register stores or replaces an event. UPSERT semantics so re-registering
// the same (eid, event_id) is idempotent.
func (s *PGStore) Register(stored *Stored) error {
	if stored == nil || stored.EID == "" || stored.EventID == "" {
		return errors.New("smds: eid and event_id are required")
	}
	if stored.RSPAddress == "" {
		return errors.New("smds: rsp_server_address is required")
	}
	if stored.RegisteredAt.IsZero() {
		stored.RegisteredAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO smds_events (eid, event_id, rsp_address, forwarding, registered_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (eid, event_id) DO UPDATE
		   SET rsp_address = EXCLUDED.rsp_address,
		       forwarding = EXCLUDED.forwarding,
		       registered_at = EXCLUDED.registered_at`,
		string(stored.EID), stored.EventID, stored.RSPAddress, stored.Forwarding, stored.RegisteredAt)
	if err != nil {
		return fmt.Errorf("smds/pg: insert: %w", err)
	}
	return nil
}

// Delete removes an event by (eid, event_id).
func (s *PGStore) Delete(eid smdsv1.EID, eventID string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM smds_events WHERE eid = $1 AND event_id = $2`,
		string(eid), eventID)
	if err != nil {
		return fmt.Errorf("smds/pg: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrEventNotFound
	}
	return nil
}

// ListForEID returns events for the given EID, oldest first.
func (s *PGStore) ListForEID(eid smdsv1.EID) []*Stored {
	rows, err := s.pool.Query(context.Background(),
		`SELECT eid, event_id, rsp_address, forwarding, registered_at
		   FROM smds_events
		  WHERE eid = $1
		  ORDER BY registered_at ASC`, string(eid))
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanAll(rows)
}

// All returns every registered event, oldest first.
func (s *PGStore) All() []*Stored {
	rows, err := s.pool.Query(context.Background(),
		`SELECT eid, event_id, rsp_address, forwarding, registered_at
		   FROM smds_events
		  ORDER BY registered_at ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanAll(rows)
}

func scanAll(rows pgx.Rows) []*Stored {
	out := make([]*Stored, 0)
	for rows.Next() {
		var (
			eid          string
			eventID      string
			rspAddr      string
			forwarding   bool
			registeredAt time.Time
		)
		if err := rows.Scan(&eid, &eventID, &rspAddr, &forwarding, &registeredAt); err != nil {
			continue
		}
		out = append(out, &Stored{
			EID:          smdsv1.EID(eid),
			EventID:      eventID,
			RSPAddress:   rspAddr,
			Forwarding:   forwarding,
			RegisteredAt: registeredAt.UTC(),
		})
	}
	return out
}
