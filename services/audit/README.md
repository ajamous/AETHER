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
| Postgres-backed ledger                            | Not started   |
| NATS subscriber                                   | Not started   |
| SSE streaming                                     | Not started   |
| Full-text search                                  | Not started   |

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
