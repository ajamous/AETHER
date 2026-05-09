# Annual recertification checklist

SAS-SM accreditation is annual. Use this checklist 60 days before
your scheduled recertification audit to confirm the evidence pack
is current. Items grouped by control family, mirroring the
[gap analysis](gap-analysis.md). Each item points at the document
or operational artifact that satisfies it.

## 60 days out: collect

### A. Security policy

- [ ] Security policy document — current version, signed
- [ ] Roles and responsibilities matrix — current
- [ ] Risk register — last reviewed within 90 days
- [ ] Threat model document — references current architecture

### B. Sensitive process

- [ ] Gateway TLS + ES2+ mTLS — Helm values diff vs production
      (see [reference-aws.md](reference-aws.md))
- [ ] Sample audit-log entries showing 401 rejections of bad
      ES2+ client certs (last 90 days)
- [ ] Sample audit-log entries showing successful ES9+ flows
      (initiateAuthentication + authenticateClient round trip
      with same transactionId, last 90 days)

### C. Key management

- [ ] Chain-of-custody form for every production key currently
      in use ([key-ceremony.md](key-ceremony.md))
- [ ] HSM model + FIPS certification PDF
- [ ] Evidence of key rotation per policy (rotation log)
- [ ] Cert expiry monitoring active —
      `aether_cert_expiry_days{...}` metric scraped, alert rule
      firing window verified
- [ ] Trust-store ConfigMap version-control history (Git log)

### D. Network and infrastructure

- [ ] TLS configuration evidence — gateway args, ingress policy
- [ ] Network policies in cluster — `kubectl get networkpolicy
      -n aether -o yaml`
- [ ] Postgres encryption at rest — RDS config screenshot or
      equivalent
- [ ] VPC Flow Logs enabled — provider screenshot

### E. Audit and logging

- [ ] `/v1/verify` returning `ok=true` — capture today's response
- [ ] Last 30 daily timeline anchors from the WORM bucket
- [ ] WORM bucket policy showing Object Lock Compliance + 3-year
      retention
- [ ] Postgres GRANT script applied —
      `\dp audit_entries` showing UPDATE/DELETE revoked from
      application role
- [ ] Last successful audit-log restore drill report

### F. Personnel and access

- [ ] Current role-assignments document
      ([rbac.md](rbac.md) §"How to map humans") — signed
- [ ] Last quarterly access review record
- [ ] Off-boarding checklist evidence for any leavers in the
      past 12 months
- [ ] Break-glass activations for the past 12 months — list, or
      attest "none"

### G. Incident management

- [ ] Postmortems for every Sev-1 / Sev-2 in the past 12 months
      (or attest "none")
- [ ] Incident response runbook current with the deployment
      ([incident-response.md](incident-response.md))
- [ ] Last tabletop exercise record
- [ ] Vulnerability disclosure log (SECURITY.md reports +
      responses)
- [ ] Patch-application records for the past 12 months

### H. Business continuity

- [ ] DR runbook current with the deployment
      ([disaster-recovery.md](disaster-recovery.md))
- [ ] RTO / RPO documented and signed off by the business
- [ ] Last quarterly RDS PITR drill report
- [ ] Last annual full S2 (regional outage) simulation report
- [ ] Backup retention configuration — RDS backup window,
      manual snapshot policy
- [ ] Last cross-region replication validation

## 30 days out: review

- [ ] Walk the [common findings](common-findings.md) list with
      your operations lead. Confirm each item's mitigation is in
      place and evidenced.
- [ ] Compare last year's auditor recommendations to this year's
      state. Closed items: link the closure evidence. Open items:
      have a status answer ready.
- [ ] Run a dry-run audit against the [gap analysis](gap-analysis.md)
      with someone who didn't write it. Anything they can't find
      in five minutes is a finding waiting to happen.

## Audit week

- [ ] Capture today's `/v1/verify` output
- [ ] Capture today's pod status:
      `kubectl get pods -n aether -o wide`
- [ ] Capture today's cert expiry dashboard
- [ ] Confirm break-glass access has not been activated in the
      past 7 days
- [ ] Confirm the on-call rota is staffed for the audit window
- [ ] Print one fresh copy of every signed form going into the
      pack — auditors prefer paper for the chain-of-custody

## After the audit

- [ ] Whatever findings the auditor raised: capture each with
      owner, due date, and severity
- [ ] Postmortem-style review with the team: what was painful
      to produce, what was easy, what should we automate before
      next year
- [ ] If you passed: consider contributing back the anonymised
      version of the findings + the items from this checklist
      that mattered most. See `index.md` §"How to contribute
      back". The goal is for the next adopter to need this
      checklist less than you did.

## Why these items in this order

The order tracks how an auditor's session typically flows: open
with policy and people (A, F), move into how the platform handles
the sensitive process (B, C, D), then audit the audit (E), then
close with what happens when something goes wrong (G, H). Knowing
the flow lets you stage your evidence pack the way the auditor
will read it.
