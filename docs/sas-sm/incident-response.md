# Incident response runbook

A SAS-SM auditor will look for: a documented severity matrix, a
documented escalation path, evidence of timely response, and a
postmortem culture. This runbook gives you the structure; the
specific names and phone numbers come from your operator-side
on-call rota.

The audit log itself ([audit-retention.md](audit-retention.md)) is
the primary forensic surface — every sensitive operation lands in
the hash-chained ledger. The runbook below tells you what to do
once you know something went wrong.

## Severity matrix

| Severity | What it means                                                  | Response target | Escalates to                   |
| -------- | -------------------------------------------------------------- | --------------- | ------------------------------ |
| Sev-1    | Active customer impact OR audit chain integrity broken OR HSM unreachable OR private key suspected exposed | 15 min ack, 1h triage | Incident commander + key custodians + auditor (if scope includes audit chain) |
| Sev-2    | Degraded service (elevated error rate, slow ES2+ responses) OR cert expiry within 7 days OR persistent 401s on ES2+ from a single BSS | 1 hour ack, 4h triage | Incident commander                |
| Sev-3    | Single-pod crash with auto-recovery, transient 5xx burst, single-cert expiry within 30 days | Next business day | On-call rota                    |
| Sev-4    | Cosmetic issues, dashboard gaps, non-customer-facing CI failures | Best effort     | Backlog                         |

The two "always Sev-1" rules:

- **Audit chain integrity broken** — `/v1/verify` returns
  `ok=false`. Even if no customer is impacted yet, the chain
  break is a SAS-SM-relevant event and the auditor will expect
  evidence you noticed and acted within the window.
- **Suspected private-key exposure** — any indication a DPauth /
  DPpb / DPtls private key may have left the HSM, or that an
  HSM PIN was compromised. The platform makes this *very* hard
  by design (see [ADR 0003](../architecture/adr/0003-pkcs11-abstraction.md)),
  but if you suspect it, treat as Sev-1.

## Roles

| Role                 | Job during the incident                                |
| -------------------- | ------------------------------------------------------ |
| Incident commander   | Owns the call. Makes decisions. Communicates.          |
| On-call engineer     | Investigates. Executes fixes.                          |
| Scribe               | Captures the timeline in the channel as it happens.    |
| Comms lead           | Drafts customer / BSS-partner / regulator updates.     |
| Key custodian (Sev-1 with key concern only) | Available for emergency rotation. |

The same person can hold multiple roles in a small operator. The
incident commander cannot also be the on-call engineer — too much
context-switching, and the auditor will note it.

## Standard playbook

For every Sev-1 and Sev-2:

### 1. Acknowledge

The first responder posts in the incident channel:

```
Sev-?, ack at HH:MM. Symptoms: <one line>. Initial impact: <users / BSS / chain>. Beginning triage.
```

This is the timestamp the auditor will reconstruct from. Get it
right.

### 2. Designate roles

Even for a small operator, name the IC and the scribe explicitly:

```
IC: alice. Scribe: bob. On-call engineer: alice (combined).
```

If `alice` is also IC and engineer, document that the merge was
deliberate (size of org) and not an oversight.

### 3. Triage

Pull the relevant evidence into the channel as you find it. Stop
talking the moment a customer-facing communication is ready to
go out.

Useful first looks:

```
# Service health
kubectl get pods -n aether
kubectl logs -n aether <smdp-plus-pod> --tail=200

# Audit chain integrity
curl http://audit.aether.svc/v1/verify

# Cert health
curl http://certmgr.aether.svc/metrics | grep aether_cert

# Recent ES9+ failures (last 30 min)
# (run via your log pipeline; sample query in CloudWatch Insights
# documented in reference-aws.md)
```

### 4. Mitigate

Mitigate before you fix. The auditor will look at the gap between
"impact starts" and "impact stops" — that's the metric.

Standard mitigations by symptom:

| Symptom                                  | Mitigation                                              |
| ---------------------------------------- | ------------------------------------------------------- |
| ES9+ returning 5xx                       | Roll smdp-plus back to last known good. `kubectl rollout undo deployment/aether-smdp-plus -n aether` |
| ES2+ returning 401 unexpectedly          | Inspect gateway client-CA secret; recent rotation likely culprit |
| Audit `/v1/verify` ok=false              | Disable writes (smdp-plus / smds / eim replicas → 0), preserve evidence, trigger DR procedure |
| Cert expiring in < 24h                   | Rotate via [key-ceremony.md](key-ceremony.md) cert-rotation flow; if no time, accept reduced trust temporarily and plan emergency rotation |
| HSM broker unreachable                   | Check HSM provider's status page; failover to standby HSM if multi-HSM deployed |
| Secret leaked or suspected compromised   | Revoke at IdP / HSM, rotate, audit-log review for use of leaked secret |

### 5. Confirm recovery

For Sev-1 / Sev-2: explicit declaration in the channel.

```
Recovery confirmed at HH:MM. Service indicators green for 15 min. Closing as Sev-?.
```

Don't close on a single green probe — wait the full 15 minutes.
The auditor will check.

### 6. Postmortem

Required for every Sev-1 and Sev-2. The plan
([philosophy principle 4](https://github.com/ajamous/aether/blob/main/GOVERNANCE.md))
says postmortems are public; specific carrier identifiers can be
redacted, but the technical narrative is published.

The format:

```
# Incident report — Sev-X — short title — YYYY-MM-DD

## Summary
1-2 sentences. What broke, who was affected, how long.

## Timeline (UTC)
HH:MM | event
HH:MM | event
...

## Impact
Concrete numbers. "X profile downloads failed" not "many".

## Root cause
Why this happened, mechanically. Not "human error" — what about
the system allowed the human to err?

## What worked
Things to keep doing.

## What didn't
Things to change. Each item has an owner and a date.

## Action items
| # | Item | Owner | Due |
| - | ---- | ----- | --- |
```

Postmortems land under `docs/operations/postmortems/` with a
filename like `2026-05-09-audit-chain-break.md`. Anonymise as
needed.

## Audit-chain-break sub-procedure (always Sev-1)

This is its own playbook because the response order is special:
**preserve evidence first, restore later.**

1. The on-call engineer confirms `/v1/verify` returns
   `ok=false` and captures the full response with timestamp.
2. **Stop writes immediately**. Scale smdp-plus, smds, eim to
   zero replicas. Do not stop the audit pod itself — it's
   read-only at this point and you want it queryable.
3. **Snapshot Postgres** before any restore. Tag the snapshot
   as forensic evidence; it will not be discarded for the
   retention period of the audit log.
4. **Compare the live tail to the daily timeline anchor**. If
   they match, the break is recent (between the last anchor
   and now). If the live tail is older than the anchor, you
   have data loss; investigate before continuing.
5. **Restore from WORM dump** following
   [disaster-recovery.md](disaster-recovery.md) §S3.
6. **Resume writes** only after `/v1/verify` returns `ok=true`
   on the restored chain.
7. **Notify auditor** and any affected regulator. SAS-SM
   accreditation requires you to disclose chain breaches even
   if no customer was directly impacted.

## Evidence to put in the audit pack

- This document.
- The severity matrix as your operator policy ratifies it (may
  diverge from the table above; the auditor wants to see your
  version).
- The on-call rota (anonymised if needed; the auditor looks for
  shape, not names).
- Postmortems for all Sev-1 / Sev-2 incidents in the past 12
  months. If you've had no Sev-1 / Sev-2 incidents, an
  attestation to that effect signed by the IC role-holder.
- Tabletop exercise records.

## What this runbook does NOT solve

- **It does not page anyone.** Hook your alerting (Pagerduty,
  Opsgenie, etc.) up to the alerts the
  [reference-aws.md](reference-aws.md) Observability section
  lists. The runbook starts at "the engineer is paged."
- **It does not draft your customer comms.** A comms-lead
  template lives in your operator-supplied playbook; this
  document covers the technical incident pipeline.
- **It does not enforce its own SLAs.** The 15-minute / 1-hour
  acknowledgement targets are policy. Track them in your
  postmortem ledger.
