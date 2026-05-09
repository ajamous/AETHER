// Postgres-backed implementation of the eIM Store.

package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	eimv1 "github.com/ajamous/aether/services/eim/api/v1"
)

// PGStore persists devices and commands in PostgreSQL.
type PGStore struct {
	pool *pgxpool.Pool
}

const pgSchemaSQL = `
CREATE TABLE IF NOT EXISTS eim_devices (
    eid           TEXT PRIMARY KEY,
    label         TEXT NOT NULL DEFAULT '',
    tags          TEXT[] NOT NULL DEFAULT '{}',
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen     TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS eim_commands (
    id            TEXT PRIMARY KEY,
    eid           TEXT NOT NULL REFERENCES eim_devices(eid) ON DELETE CASCADE,
    kind          TEXT NOT NULL,
    smdp_address  TEXT NOT NULL DEFAULT '',
    matching_id   TEXT NOT NULL DEFAULT '',
    iccid         TEXT NOT NULL DEFAULT '',
    state         TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at  TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    failure_code  TEXT NOT NULL DEFAULT '',
    failure_note  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS eim_commands_eid_state_idx
  ON eim_commands (eid, state);
`

// NewPGStore opens a pool, applies the schema, and returns a store.
func NewPGStore(ctx context.Context, url string) (*PGStore, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("eim/pg: connect: %w", err)
	}
	if _, err := pool.Exec(ctx, pgSchemaSQL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("eim/pg: migrate: %w", err)
	}
	return &PGStore{pool: pool}, nil
}

// Close releases the pool.
func (s *PGStore) Close() { s.pool.Close() }

// --- devices --------------------------------------------------------------

func (s *PGStore) RegisterDevice(d *eimv1.Device) error {
	if d == nil || d.EID == "" {
		return ErrInvalidArgument
	}
	if d.RegisteredAt.IsZero() {
		d.RegisteredAt = time.Now().UTC()
	}
	meta, err := json.Marshal(d.Metadata)
	if err != nil {
		return fmt.Errorf("eim/pg: marshal metadata: %w", err)
	}
	if d.Tags == nil {
		d.Tags = []string{}
	}
	if string(meta) == "null" {
		meta = []byte("{}")
	}
	_, err = s.pool.Exec(context.Background(),
		`INSERT INTO eim_devices (eid, label, tags, metadata, registered_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5)`,
		string(d.EID), d.Label, d.Tags, string(meta), d.RegisteredAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDeviceExists
		}
		return fmt.Errorf("eim/pg: register: %w", err)
	}
	return nil
}

func (s *PGStore) GetDevice(eid eimv1.EID) (*eimv1.Device, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT eid, label, tags, metadata, registered_at, last_seen
		   FROM eim_devices WHERE eid = $1`, string(eid))
	d, err := scanDevice(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDeviceNotFound
		}
		return nil, err
	}
	return d, nil
}

func (s *PGStore) ListDevices() []*eimv1.Device {
	rows, err := s.pool.Query(context.Background(),
		`SELECT eid, label, tags, metadata, registered_at, last_seen
		   FROM eim_devices ORDER BY registered_at ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]*eimv1.Device, 0)
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			continue
		}
		out = append(out, d)
	}
	return out
}

func (s *PGStore) DeleteDevice(eid eimv1.EID) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM eim_devices WHERE eid = $1`, string(eid))
	if err != nil {
		return fmt.Errorf("eim/pg: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDeviceNotFound
	}
	return nil
}

// --- commands -------------------------------------------------------------

func (s *PGStore) EnqueueCommand(c *eimv1.Command) error {
	if c == nil || c.EID == "" || c.Kind == "" {
		return ErrInvalidArgument
	}
	if _, err := s.GetDevice(c.EID); err != nil {
		return err
	}
	if c.ID == "" {
		c.ID = newCommandID()
	}
	if c.State == "" {
		c.State = eimv1.CommandStatePending
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO eim_commands (id, eid, kind, smdp_address, matching_id, iccid, state, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		c.ID, string(c.EID), string(c.Kind), c.SMDPAddress, c.MatchingID, c.ICCID, string(c.State), c.CreatedAt)
	if err != nil {
		return fmt.Errorf("eim/pg: enqueue: %w", err)
	}
	return nil
}

func (s *PGStore) GetCommand(id string) (*eimv1.Command, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, eid, kind, smdp_address, matching_id, iccid, state, created_at, delivered_at, completed_at, failure_code, failure_note
		   FROM eim_commands WHERE id = $1`, id)
	c, err := scanCommand(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCommandNotFound
		}
		return nil, err
	}
	return c, nil
}

func (s *PGStore) ListCommandsForDevice(eid eimv1.EID, includeCompleted bool) []*eimv1.Command {
	q := `SELECT id, eid, kind, smdp_address, matching_id, iccid, state, created_at, delivered_at, completed_at, failure_code, failure_note
	        FROM eim_commands WHERE eid = $1`
	if !includeCompleted {
		q += ` AND state IN ('pending', 'delivered')`
	}
	q += ` ORDER BY created_at ASC`
	rows, err := s.pool.Query(context.Background(), q, string(eid))
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := make([]*eimv1.Command, 0)
	for rows.Next() {
		c, err := scanCommand(rows)
		if err != nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (s *PGStore) MarkDelivered(id string) error {
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE eim_commands
		    SET state = 'delivered', delivered_at = now()
		  WHERE id = $1 AND state = 'pending'`, id)
	if err != nil {
		return fmt.Errorf("eim/pg: mark delivered: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either not found or already past pending — both OK on MarkDelivered.
		// Verify it exists at all so the caller learns about a typo'd ID.
		if _, err := s.GetCommand(id); err != nil {
			return err
		}
	}
	return nil
}

func (s *PGStore) AckCommand(id string, req *eimv1.AckCommandRequest) error {
	if req == nil || (req.State != eimv1.CommandStateCompleted && req.State != eimv1.CommandStateFailed) {
		return ErrInvalidArgument
	}
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE eim_commands
		    SET state = $2, completed_at = now(),
		        failure_code = $3, failure_note = $4
		  WHERE id = $1`,
		id, string(req.State), req.FailureCode, req.FailureNote)
	if err != nil {
		return fmt.Errorf("eim/pg: ack: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrCommandNotFound
	}
	// Touch the device's last_seen.
	if c, err := s.GetCommand(id); err == nil {
		_, _ = s.pool.Exec(context.Background(),
			`UPDATE eim_devices SET last_seen = now() WHERE eid = $1`, string(c.EID))
	}
	return nil
}

// --- helpers --------------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDevice(r rowScanner) (*eimv1.Device, error) {
	var (
		eid       string
		label     string
		tags      []string
		metaRaw   []byte
		regAt     time.Time
		lastSeen  *time.Time
	)
	if err := r.Scan(&eid, &label, &tags, &metaRaw, &regAt, &lastSeen); err != nil {
		return nil, err
	}
	var meta map[string]any
	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &meta)
	}
	return &eimv1.Device{
		EID: eimv1.EID(eid), Label: label, Tags: tags,
		Metadata: meta, RegisteredAt: regAt.UTC(), LastSeen: lastSeen,
	}, nil
}

func scanCommand(r rowScanner) (*eimv1.Command, error) {
	var (
		id, eid, kind, addr, matching, iccid, state, fcode, fnote string
		created                                                   time.Time
		delivered                                                 *time.Time
		completed                                                 *time.Time
	)
	if err := r.Scan(&id, &eid, &kind, &addr, &matching, &iccid, &state, &created, &delivered, &completed, &fcode, &fnote); err != nil {
		return nil, err
	}
	return &eimv1.Command{
		ID: id, EID: eimv1.EID(eid), Kind: eimv1.CommandKind(kind),
		SMDPAddress: addr, MatchingID: matching, ICCID: iccid,
		State: eimv1.CommandState(state), CreatedAt: created.UTC(),
		DeliveredAt: delivered, CompletedAt: completed,
		FailureCode: fcode, FailureNote: fnote,
	}, nil
}

// isUniqueViolation detects Postgres SQLSTATE 23505 (PK collision).
func isUniqueViolation(err error) bool {
	type sqlStater interface{ SQLState() string }
	var s sqlStater
	if errors.As(err, &s) {
		return s.SQLState() == "23505"
	}
	return false
}
