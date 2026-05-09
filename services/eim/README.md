# services/eim

Aether's eSIM IoT Manager (eIM). Implements the operator-facing
surface of GSMA SGP.32: a registry of IoT devices keyed by EID, plus
a per-device command queue that IoT Profile Assistants (IPAs) poll.

The eIM is the *operator's* control point for an IoT fleet: it knows
which devices exist, which profiles are pending for each, and what
each device has reported back. The IPA on each device is a separate
codebase (typically vendor-supplied); we don't ship one.

## Status

| Piece                                            | Status        |
| ------------------------------------------------ | ------------- |
| Device registry                                  | Implemented (in-memory + Postgres) |
| Command queue per device                         | Implemented (in-memory + Postgres) |
| HTTP API: register / list / fetch / deregister   | Implemented   |
| HTTP API: enqueue / list / ack commands          | Implemented   |
| IPAd flow (direct profile fetch via SM-DP+)      | Skeleton — uses command queue today |
| IPAe flow (indirect, via the eIM as relay)       | Not started   |
| ES_eIM_Device authenticated transport            | Not started — bearer token placeholder |
| Bulk operations (mass profile assignment)        | Not started   |
| Push notification to devices                     | Not started   |

The skeleton models the SGP.32 control plane: an operator (via UI or
BSS) enqueues a command for a device, and the IPA fetches it on its
next poll. The actual SGP.32 cryptographic transport (eIM ↔ IPA
authentication and command authentication) is the next step on this
service.

## Endpoints

### Operator side

| Method | Path                                   | Purpose |
| ------ | -------------------------------------- | ------- |
| POST   | `/v1/devices`                          | Register a new device |
| GET    | `/v1/devices`                          | List registered devices |
| GET    | `/v1/devices/{eid}`                    | Fetch one device |
| DELETE | `/v1/devices/{eid}`                    | Deregister |
| POST   | `/v1/devices/{eid}/commands`           | Queue a command |
| GET    | `/v1/devices/{eid}/commands`           | List pending commands for the device |
| GET    | `/v1/health`                           | Liveness |

### IPA side

| Method | Path                                                    | Purpose |
| ------ | ------------------------------------------------------- | ------- |
| GET    | `/v1/ipa/{eid}/poll`                                    | IPA polls for pending commands |
| POST   | `/v1/ipa/{eid}/commands/{command_id}/ack`               | IPA acks a command (success / failure) |

## Command shape

```json
{
  "id": "01HKZN6XRQ...",
  "kind": "download_profile" | "enable_profile" | "disable_profile" | "delete_profile",
  "smdp_address": "smdp.example",
  "matching_id": "ABC-123",
  "iccid": "8901234567890123456",
  "created_at": "...",
  "state": "pending" | "delivered" | "completed" | "failed"
}
```

## Storage modes

The default is the in-memory store — fine for the lab. Pass
`--pg-url` (or set `AETHER_PG_URL`) to use Postgres. Schema is
applied on startup.

## Running

```
go run ./cmd/eim --listen=:8449
```

The lab Docker Compose wires the eIM with the in-cluster Postgres.
