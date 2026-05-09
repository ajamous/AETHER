# Audit log retention defaults

`services/audit` is a hash-chained append-only ledger of every
sensitive operation in Aether. SAS-SM auditors will look at:

1. **What is logged** — does the platform record every sensitive
   operation, or just some of them?
2. **How long it's kept** — is the retention period documented and
   enforced?
3. **How tampering is detected** — can you prove no row has been
   modified since it was written?
4. **Who can read it** — is read access logged and segregated from
   write access?

This document tells you the platform defaults for each, and what
operator policy you need to add on top.

## What is logged

The audit log captures, at a minimum:

| Event                                       | Source                          |
| ------------------------------------------- | ------------------------------- |
| ES2+ DownloadOrder / ConfirmOrder / Cancel  | `services/gateway`              |
| ES9+ initiateAuthentication                 | `services/smdp-plus`            |
| ES9+ authenticateClient (success and fail)  | `services/smdp-plus`            |
| ES9+ getBoundProfilePackage                 | `services/smdp-plus` (when BPP lands) |
| HSM Sign / DeriveKey / GenerateKeyPair      | `services/hsm-broker`           |
| Cert chain load / verify                    | `services/certmgr`              |
| Cert rotation                               | `services/certmgr`              |
| SM-DS event registration / deletion         | `services/smds`                 |
| eIM device register / deregister / command  | `services/eim`                  |
| Operator UI sign-in / sign-out              | `ui/admin` via Auth.js (OIDC delegated to your IdP) |
| Configuration changes                       | Operator-supplied (Helm history)|

Each event is one row in `audit_entries`:

```
seq        BIGSERIAL  PRIMARY KEY
ts         TIMESTAMPTZ
payload    BYTEA      -- exact bytes the application emitted
prev_hash  BYTEA      -- hash of the previous row
hash       BYTEA      -- SHA-256(seq || ts.UnixNano || payload || prev_hash)
```

The `payload` is `BYTEA`, not `JSONB`, deliberately. JSONB
normalises whitespace and key order on round-trip; the chain
hashes exact bytes, so JSONB normalisation would break the chain.
The application validates JSON before INSERT but stores raw bytes.

## Default retention

| Event class     | Retention | Storage profile |
| --------------- | --------- | --------------- |
| Sensitive process events (ES2+, ES9+, HSM, certmgr) | 3 years immutable | Postgres + offsite backup, both encrypted at rest |
| Configuration changes | 3 years | Helm release history + Git |
| Operator UI access | 1 year | Centralised log (Loki / equivalent) |
| Service stdout/stderr (non-audit) | 30 days | Centralised log |

3 years is the SAS-SM Standard's typical floor for sensitive
process records. Some jurisdictions require longer; check yours
and configure accordingly.

## Backup and offsite copy

The hash-chained ledger only proves that no row has been modified
*on the live database*. To survive a database compromise, you
need an offsite copy:

1. Nightly `pg_dump` of `audit_entries` to encrypted object storage.
2. WORM (Write-Once-Read-Many) bucket policy on the destination.
   AWS S3 Object Lock with `Compliance` mode is the canonical
   choice. Equivalent on GCP: Bucket Lock; on Azure: immutable
   blob storage.
3. Cross-region replication of the WORM bucket.
4. Quarterly restore drill.

Auditor will want:

- WORM bucket policy showing the retention period and
  legal-hold-by-default.
- Last successful restore drill report.
- Verification that the restored ledger's chain still verifies
  via `/v1/verify`.

## Tamper detection

The chain itself detects tampering at row level:

```bash
# Live verification — anyone can run this without write
# permissions.
curl http://audit.aether.svc/v1/verify
# {"ok":true,"length":47281}
```

If anyone tampers with any row, every row from that point forward
breaks the chain, and `/v1/verify` returns `ok=false` with the
specific `failed_at_seq` and reason.

We test this in CI: `TestPGLedger_TamperDetected` directly UPDATEs
a row in Postgres and confirms `Verify()` reports the right break.

For the audit pack, schedule:

- Hourly: `/v1/verify` from a monitor service. Page on `ok=false`.
- Daily: a cron job that records the day's `length` and the
  current tail `hash` to a file appended to your immutable bucket.
  This creates an external timeline anchor — even if the entire
  Postgres is rebuilt from scratch, you can prove what the chain
  looked like on each day.

## Read access segregation

The Postgres GRANTs from [rbac.md](rbac.md#postgres-grants) give
auditors read-only access to `audit_entries` via a separate
`aether_auditor` role. That role:

- Cannot UPDATE, DELETE, or TRUNCATE any table.
- Connects via a separate connection string with its own password.
- Its connections are logged separately by Postgres.

The auditor uses this role through a read-only proxy if they want
to query the live ledger. Most auditors prefer the offsite copy.

## What to put in the audit pack

- This document.
- The retention policy in your operator-supplied policy doc, with
  any jurisdiction-specific extensions.
- The WORM bucket policy.
- The last restore drill report.
- The latest `/v1/verify` output (`ok=true`).
- The Postgres GRANT script from [rbac.md](rbac.md).
- A 30-day sample of `aether_auditor` Postgres connection logs.

## What this default does NOT solve

- Storage cost optimisation. 3 years of dense audit log can be
  large. Cold-storage tiering after, say, 90 days is operator-
  supplied.
- Your jurisdiction's data residency requirements. If the
  Standard or your regulator says "keep within country X," that
  constrains your bucket region; the platform doesn't know
  about it.
- Search and discovery over the log. The current API supports
  list-since-seq and per-row fetch; full-text search lands in a
  follow-up. For now, an auditor querying for "all events for
  EID X between dates" goes through Postgres directly via the
  `aether_auditor` role.
