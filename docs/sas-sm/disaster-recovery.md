# Disaster recovery runbook

A SAS-SM auditor will look for: a documented DR plan, defined RTO
and RPO, evidence of regular drills, and operational confidence
that the audit chain survives a regional event. This runbook gives
you the procedure; the policy targets (RTO, RPO, geography) come
from your business.

## What "disaster" means here

Three escalating scenarios:

| # | Scenario                                  | Practical meaning                                |
| - | ----------------------------------------- | ------------------------------------------------ |
| 1 | Single-AZ failure                         | One AZ goes dark; cluster keeps running on the others |
| 2 | Regional service outage                   | Whole AWS/GCP region unreachable for hours       |
| 3 | Database compromise or destruction        | Postgres data gone or known-tampered             |

Aether's default reference deployment ([reference-aws.md](reference-aws.md))
absorbs (1) without operator intervention thanks to RDS Multi-AZ +
EKS multi-AZ nodegroup. (2) and (3) need this runbook.

## RTO / RPO targets

These are starting points; tune to your business.

| Service             | RTO   | RPO   | Driver                                  |
| ------------------- | ----- | ----- | --------------------------------------- |
| `services/audit`    | 4h    | 24h   | Hash chain + WORM offsite copy          |
| `services/smds`     | 4h    | 1h    | Live SM-DP+ ↔ device discovery flow     |
| `services/eim`      | 4h    | 1h    | IoT command-queue freshness             |
| `services/smdp-plus` sessions | 1h | 5m | Active LPA flows reconnect on retry     |
| `services/certmgr`  | 1h    | n/a   | PKI is in HSM/Secrets, not Postgres     |
| `services/hsm-broker` | 1h  | n/a   | Stateless once HSM is up                |
| `ui/admin`          | 1h    | n/a   | Stateless                                |
| `services/profile-builder` | 1h | n/a | YAML templates in Git                  |

The audit log's RPO is deliberately the slackest: 24h matches a
nightly WORM dump cycle. Tightening it to "minutes" requires
streaming replication of the audit ledger to a secondary region,
which is a Phase 6 follow-up. The audit chain still detects any
gap that landed between the last anchor and the failure.

## Backup architecture

```
   Postgres (primary)         Cluster events
          │                        │
          ▼                        ▼
   pg_basebackup            Audit /v1/verify
   (continuous WAL)         Daily timeline anchor
          │                        │
          ▼                        ▼
       RDS auto-          S3 Object Lock (Compliance)
       backup (35d)       cross-region replicated
```

- **RDS automated backups** carry 35 days of WAL → 35-day PITR.
- **Manual snapshots** before every Aether minor-version upgrade
  ride next to the auto-backups.
- **Audit log offsite copy**: a CronJob runs `pg_dump
  audit_entries` nightly and writes the dump (zstd-compressed) to
  the WORM bucket. The bucket policy is Object Lock Compliance
  with a 3-year retention default — even the bucket owner cannot
  shorten it. Cross-region replication carries the dump to a
  second qualifying region.
- **Daily timeline anchor**: a separate CronJob (or the same one)
  records `(date, last_seq, last_hash)` from `/v1/verify` to a
  second WORM object. This creates an external integrity timeline
  that survives a complete database rebuild.

## Recovery procedures

### S1 — Single-AZ failure

Nothing. RDS Multi-AZ promotes the standby; EKS reschedules pods
to surviving AZs. The on-call engineer confirms with:

```
kubectl get pods -n aether
aws rds describe-db-instances --db-instance-identifier aether-prod
```

Audit pipeline check: `curl http://audit/v1/verify` reports
`ok=true`. Total time: < 5 minutes.

### S2 — Regional outage

Recovery target: a parallel deployment in the secondary qualifying
region.

1. **Confirm the outage is real**, not a control-plane blip.
   Check the cloud's status page; a 5-minute glitch is not a DR
   event.
2. **Promote the secondary RDS** (cross-region read replica, if
   you maintain one) or restore from the latest WORM dump.
3. **Stand up Aether in the secondary region** using the same
   Helm values with the new RDS endpoint. The existing
   [reference-aws.md](reference-aws.md) describes the topology;
   apply it in the secondary region.
4. **Re-issue or rotate the public endpoint** so BSS systems
   reach the new gateway. Update DNS or your traffic-shaping
   layer.
5. **Audit chain integrity check** in the new region.
   `curl /v1/verify` must return `ok=true`. If `length` is
   smaller than the last-known anchor's `seq`, the dump
   restored from is older than expected — investigate before
   accepting any new traffic.
6. **Postgres lag check** for SM-DS, eIM, smdp-plus session
   stores. Stale data in these is more tolerable than in the
   audit chain because of their RPO targets, but the operator
   should know what's lost.

Total time, with an operator on shift and infrastructure ready:
2–4 hours. Most of that is DNS propagation and Helm install.

### S3 — Database compromise

The chain's tamper detection is the gate.

1. **Run `/v1/verify` immediately.** If `ok=false`, you know
   which seq broke. Capture the response in evidence.
2. **Suspend all traffic** at the gateway via maintenance mode
   (set `replicaCount: 0` on smdp-plus and smds). Do not delete;
   you want the existing pods' state for forensics.
3. **Snapshot the live (compromised) Postgres** before any
   restore. Tag it as evidence. Hand to forensics if relevant.
4. **Restore from the most recent known-good WORM dump.**
   "Known-good" means: its tail hash matches the timeline anchor
   for that date. If no anchor matches, walk backwards anchor by
   anchor until one does. Document which anchor you trusted and
   why.
5. **Cross-check** the restored chain end-to-end with
   `/v1/verify`. `ok=true` is mandatory before proceeding.
6. **Resume traffic.** SM-DS / smdp-plus / eIM state between the
   restore point and now is lost; the platform's RPO budget
   covers the time gap, but communicate with affected BSS
   systems if your SLAs are tighter than the RPO.

Total time: 4–8 hours, depending on how much manual anchor-
walking is needed.

## Drill cadence

| Drill                                 | Frequency  |
| ------------------------------------- | ---------- |
| RDS PITR restore to a side cluster    | Quarterly  |
| Full S2 simulation (cold-start in DR region) | Annually |
| `/v1/verify` cron alert exercise      | Monthly    |
| WORM-bucket dump-restore validation   | Monthly    |
| Tabletop exercise on the runbook      | Annually   |

The auditor will ask to see the **last** of each. Keep the
records.

## Evidence to put in the audit pack

- This document, with the operator's RTO/RPO numbers filled in.
- Drill records for the past 12 months: dates, who ran them,
  what worked, what didn't.
- The most recent successful restore drill report: snapshot ID,
  restore-target details, `/v1/verify` output, time taken.
- The WORM bucket lifecycle policy showing Compliance mode and
  cross-region replication.
- Daily timeline anchor objects from the WORM bucket — at least
  the last 30 days.

## What this runbook does NOT solve

- **It does not give you a multi-region active-active deployment.**
  That is a Phase 6 platform follow-up. Until then, S2 recovery
  is cold-start, not seamless.
- **It does not configure your RDS read replicas or
  cross-region replication.** Those are operator IaC concerns.
  The reference-aws.md topology lists them but the Helm chart
  cannot manage RDS configuration.
- **It does not handle your DNS / traffic-shaping migration.**
  The right tool depends on your front door (Route 53, Cloudflare,
  etc.); document yours alongside this runbook.
- **It does not tell you when to declare a disaster.** That call
  belongs to your incident commander, see
  [incident-response.md](incident-response.md).
