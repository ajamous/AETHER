package chain

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Append runs in serializable so concurrent appends can't both read
// the same tail row, both compute against the same prev_hash, and
// both insert with mismatched seqs. Postgres detects the
// serialization conflict; we retry on it.
var pgxBeginSerializable = pgx.TxOptions{IsoLevel: pgx.Serializable}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func isSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "40001"
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntry(r rowScanner) (*Entry, error) {
	var (
		seq      int64
		ts       time.Time
		payload  []byte
		prevHash []byte
		hash     []byte
	)
	if err := r.Scan(&seq, &ts, &payload, &prevHash, &hash); err != nil {
		return nil, err
	}
	return &Entry{
		Seq:       uint64(seq),
		Timestamp: ts.UTC(),
		Payload:   json.RawMessage(payload),
		PrevHash:  prevHash,
		Hash:      hash,
	}, nil
}
