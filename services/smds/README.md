# services/smds

Aether's Subscription Manager — Discovery Service. Implements GSMA
SGP.22 §5.5 (ES11, ES12). Two-sided service:

- **ES12** is the SM-DP+ → SM-DS surface. SM-DP+ registers a "there
  is a pending profile for EID X" event; later it can delete the
  event when the profile has been downloaded or cancelled.
- **ES11** is the LPA → SM-DS surface. The LPA polls the SM-DS for
  events pending against its EID; if anything is found, the LPA is
  told which SM-DP+ address to talk to next.

A working SM-DS unlocks zero-touch profile activation: the carrier
issues a profile and the device finds it without the user typing an
activation code or scanning a QR.

## Status

| Piece                                            | Status        |
| ------------------------------------------------ | ------------- |
| ES12 RegisterEvent (§5.5.1)                      | Implemented (in-memory) |
| ES12 DeleteEvent (§5.5.2)                        | Implemented   |
| ES11 AuthenticateClient (§5.5.4)                 | Skeleton (no signature verification yet) |
| ES11 GetEvents (§5.5.3)                          | Implemented (returns matching events for an EID) |
| Root SM-DS role                                  | Implemented (single-tier)  |
| Alternative SM-DS / cascade                      | Not started   |
| Postgres-backed event store                      | Implemented   |
| HTTPS + mTLS                                     | Not started   |
| Push notification (vs polling) channel           | Not started   |

Storage: the in-memory store is the default. Pass `--pg-url` (or set
`AETHER_PG_URL`) to use Postgres. The `(eid, event_id)` primary key
plus an `ON CONFLICT DO UPDATE` clause keeps registration idempotent.

## How a discovery handshake looks

```
SM-DP+ (just took a DownloadOrder for EID X)
  └─ POST /gsma/rsp2/es12/registerEvent {eid: X, smdp_address: aether.local}

LPA (boots, knows its EID is X)
  └─ POST /gsma/rsp2/es11/authenticateClient {euicc_challenge: ...}
     ↳ session bound to LPA's EID
  └─ POST /gsma/rsp2/es11/getEvents {transaction_id: ...}
     ↳ {"events":[{"smdp_address":"aether.local","matching_id":...}]}
```

The LPA then proceeds to that SM-DP+ via the normal ES9+ flow.

## Running

```
go run ./cmd/smds --listen=:8448
```

The lab Docker Compose wires the SM-DP+ to register against this
service and exposes :8448 to the host.
