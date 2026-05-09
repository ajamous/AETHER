# services/audit

Append-only, hash-chained event log for Aether. Every ES2+, ES8+, ES9+
call, every cert use, and every operator action in the UI lands here.
Designed to satisfy SAS-SM "Sensitive Process Data Management"
requirements.

## Status

| Piece                                            | Status        |
| ------------------------------------------------ | ------------- |
| In-memory hash-chained ledger                    | Implemented   |
| HTTP API: append, list, verify                   | Implemented   |
| Postgres-backed ledger                            | Implemented   |
| Signed timeline anchors (`/v1/anchor`)           | Implemented (unsigned by default; opt-in HSM signing via `--hsm-broker` + `--anchor-key`. ECDSA-SHA-256 over DER-encoded Anchor SEQUENCE; auditors verify offline against the published audit-anchor public key. See [docs/sas-sm/audit-retention.md](../../docs/sas-sm/audit-retention.md)) |
| NATS subscriber                                   | Not started   |
| SSE streaming                                     | Not started   |
| Full-text search                                  | Not started   |

## Storage modes

The default is the in-memory ledger — fine for unit tests and the
local lab. Pass `--pg-url` (or set `AETHER_PG_URL`) to use the
Postgres-backed ledger:

```
go run ./cmd/audit --pg-url=postgres://aether:aether@localhost:5432/aether
```

The schema is created on startup if missing. The `payload` column is
`BYTEA`, not `JSONB`, because the hash chain is over exact bytes and
JSONB normalises whitespace. Concurrent appends use serializable
transactions; conflicts retry transparently. A `TestPGLedger_Concurrent
AppendsKeepChainIntact` test runs 8 writers × 25 appends each through
the chain to lock that contract.

For SAS-SM deployments, follow the schema comment and `REVOKE UPDATE,
DELETE ON audit_entries FROM aether_app` so even an application-level
bug can't tamper with the chain.

## How the chain works

Each entry contains:

- a monotonic sequence number
- the event payload (arbitrary JSON)
- a timestamp
- the previous entry's hash
- this entry's hash, computed as
  `SHA-256(seq || timestamp || payload || prev_hash)`

`/v1/verify` walks the chain, recomputes each hash, and reports the
first inconsistency (if any). Tampering with any entry breaks every
hash from that point forward.

## Endpoints

- `POST /v1/events` — append an event (JSON body)
- `GET  /v1/events` — list events with optional `?since=N`
- `GET  /v1/events/{seq}` — fetch one event
- `GET  /v1/verify` — walk the chain, report integrity status
- `GET  /v1/health` — readiness
