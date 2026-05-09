# Common SAS-SM audit findings (and how Aether's defaults pre-empt them)

This is the document an MVNO would have wanted before their first
audit. Real auditor findings cluster around a handful of recurring
themes; the platform's defaults already address most of them, but
only if you operate them correctly. Below: the common findings, the
Aether feature that pre-empts each, and the operator-side gotcha
that could still catch you out.

> **Honest note**: this catalogue is grounded in published SAS-SM
> requirements and observable patterns. It is not an auditor-blessed
> document. Adopters who go through audits are encouraged to
> contribute back the findings their auditor raised, anonymised, so
> the section gets sharper. See `index.md` §"How to contribute back".

## Key management

### F1. "Production private keys can leave the HSM."

The single most common Sev-1 finding. Auditors look for a
documented architecture where private keys are HSM-resident and
never extractable.

- **Aether default**: `services/hsm-broker` exposes no `Export`
  operation. `Sign` returns a signature; `DeriveKey` returns a
  handle to derived bytes; `GenerateKeyPair` returns a public
  point and an opaque ID. The architecture is enforced by the
  contract, documented in [ADR 0003](../architecture/adr/0003-pkcs11-abstraction.md).
- **Operator gotcha**: PKCS#11 attribute templates on the HSM
  itself can override this — if an admin marks a key
  `CKA_EXTRACTABLE=true` outside Aether, the platform won't
  catch it. The [key-ceremony.md](key-ceremony.md) procedure
  produces keys with `CKA_EXTRACTABLE=false`; your evidence pack
  should show that attribute on every production key.

### F2. "No documented key ceremony."

Auditors will ask for the ceremony record for each production
key. Missing or hand-wavy records are a finding even if the keys
themselves are correctly stored.

- **Aether default**: [key-ceremony.md](key-ceremony.md) with the
  tear-out chain-of-custody form is the structure auditors want
  to see.
- **Operator gotcha**: signing the form is not optional. A
  ceremony where the form was filled out but not signed by both
  custodians and the scribe is a finding.

### F3. "No two-person rule on HSM access."

Single-custodian access patterns get flagged. The auditor wants
to see that no one human can authenticate to the HSM alone.

- **Aether default**: the ceremony procedure mandates a
  two-custodian quorum. The HSM-side enforcement (e.g. CloudHSM
  CO + USR PIN split, Luna M-of-N) is operator-supplied — the
  platform records the ceremony but doesn't enforce the PIN
  policy on the HSM itself.
- **Operator gotcha**: in lab mode, the SoftHSM PIN is a single
  string. Don't drag that mental model into production.

## Audit logging

### F4. "Audit log is not provably append-only."

Auditors will try to `UPDATE` or `DELETE` an `audit_entries` row,
or look for grants that would allow it.

- **Aether default**: the application code never UPDATEs or
  DELETEs `audit_entries`. The hash chain detects tampering even
  if someone gets in via raw SQL.
- **Operator gotcha**: not the same as the database refusing it.
  Until you `REVOKE UPDATE, DELETE, TRUNCATE ON audit_entries
  FROM aether` (the application role), the auditor will flag
  this. The SQL is in [rbac.md](rbac.md) §"Postgres GRANTs".

### F5. "No external integrity anchor for the audit log."

Hash chains detect tampering, but they don't detect "the entire
chain was rebuilt from scratch and an attacker's tail hash was
written over yours." The auditor will ask how you'd know.

- **Aether default**: the daily timeline anchor pattern in
  [audit-retention.md](audit-retention.md) writes
  `(date, last_seq, last_hash)` to the WORM bucket every day.
  Even a complete chain rebuild can't reconstruct yesterday's
  anchor, so a discrepancy proves the rebuild.
- **Operator gotcha**: don't put the anchor in the same bucket
  as the dump itself. The whole point is they're separately
  trusted. A second WORM bucket, ideally cross-region replicated
  to a different cloud, satisfies this best.

### F6. "Audit retention policy is undocumented or inconsistent
with regulation."

3 years is a baseline; many jurisdictions require longer.

- **Aether default**: [audit-retention.md](audit-retention.md)
  states 3 years as the platform default and labels it a floor,
  not a ceiling.
- **Operator gotcha**: your jurisdiction's data-residency rule
  might constrain the bucket region. The platform doesn't know
  about your regulator; document the retention period, the
  bucket region, and the regulatory reference together.

## Network and access

### F7. "ES2+ accepts unauthenticated requests."

Most common gateway finding. SGP.22 §5.4 mandates mTLS on ES2+.

- **Aether default**: when `--es2plus-client-ca` is set on the
  gateway, requests to `/gsma/rsp2/es2plus/*` without a verified
  client cert get 401. The path-scoped mTLS is verified by an
  integration test in `services/gateway`.
- **Operator gotcha**: the lab default is plain HTTP. The
  startup banner warns about this. The auditor will check it
  isn't the production state — show them your Helm values
  override with `gateway.tls.es2plusClientCASecret` populated.

### F8. "Operator UI has no authentication."

Same family as F7, on the human side.

- **Aether default**: when the OIDC env vars are set, every page
  is gated. The lab default (no OIDC env) renders an unmissable
  "AUTH DISABLED" banner so the running state is obvious from a
  screenshot. The Helm chart wires the env from Secrets.
- **Operator gotcha**: the lab banner does not authenticate
  anyone — it just announces the gap. If your screenshot in the
  audit pack shows that banner, the auditor will ask why.

### F9. "Service-to-service traffic is unauthenticated."

Auditors increasingly expect mesh-level mTLS or NetworkPolicy
isolation for inter-service traffic, not just public ingress.

- **Aether default**: the chart's pod security context
  (non-root, read-only root FS, dropped capabilities) plus
  ClusterIP-only services give you the floor.
- **Operator gotcha**: the platform doesn't ship a service
  mesh, NetworkPolicies, or Istio config. Add Linkerd / Istio
  / NetworkPolicy as part of your cluster setup, and reference
  them in the gap-analysis evidence.

## Personnel and access

### F10. "No segregation of duties between operators and key
custodians."

The same human appearing in both lists is the finding.

- **Aether default**: [rbac.md](rbac.md) defines the four
  roles, with explicit constraints documented for the
  custodian / operator separation.
- **Operator gotcha**: the constraint is policy, not platform
  enforcement. If your IdP groups put the same human in both,
  Aether will let them in. Quarterly review of the role
  assignments is the working control; document it in the
  audit pack.

### F11. "No quarterly access review."

Even with separation in place, the auditor expects evidence
of *recent* review.

- **Aether default**: [rbac.md](rbac.md) §"Quarterly review"
  gives you the checklist.
- **Operator gotcha**: the review must be signed by someone
  other than the people whose access is being reviewed. A
  reviewer signing their own access is a finding.

## Cryptographic primitives

### F12. "Weak or unspecified curve / hash on identity certs."

Auditors will check the cert chain shape.

- **Aether default**: the platform mints DPauth keys with
  ECDSA P-256 + SHA-256 by default, matching SGP.22 §H.5. The
  HSM-side cert generation flow (operator's CSRs to a CI)
  requests the same.
- **Operator gotcha**: your CI may issue back a cert with a
  different signature algorithm than your CSR requested.
  Verify before loading into certmgr.

### F13. "TLS configuration permits weak cipher suites or old
versions."

The gateway's `MinVersion: tls.VersionTLS12` is the floor.

- **Aether default**: the gateway TLS config in
  `services/gateway/internal/tlsconf` pins TLS 1.2 minimum.
- **Operator gotcha**: most cloud LBs sit in front of the
  gateway and terminate TLS themselves. The LB's policy is
  what matters externally; reference-aws.md sets ALB to
  `ELBSecurityPolicy-TLS13-1-2-2021-06`, but your config is
  what counts.

## Operations and continuity

### F14. "No documented disaster recovery plan."

Asks like "what's your RTO?" with no answer is a finding.

- **Aether default**: [disaster-recovery.md](disaster-recovery.md)
  with starter RTO/RPO numbers per service.
- **Operator gotcha**: the platform's RTO/RPO numbers are
  starting points — your business may need tighter. The
  auditor will ask whose policy the numbers come from. If the
  answer is "Aether's," that's not a sufficient answer.

### F15. "DR drill records missing or older than 12 months."

The runbook is necessary but not sufficient. Drill evidence is
what auditors actually score on.

- **Aether default**: [disaster-recovery.md](disaster-recovery.md)
  prescribes a quarterly RDS PITR drill and an annual full S2
  simulation.
- **Operator gotcha**: drills run by the same person who
  designed the system are weaker evidence. Rotate the drill
  driver across operators.

### F16. "No incident postmortems published."

Even "we had no incidents" needs to be attested. Silence is the
finding.

- **Aether default**: [incident-response.md](incident-response.md)
  mandates a postmortem for every Sev-1 / Sev-2.
- **Operator gotcha**: a Sev-1 with no postmortem is treated as
  the worst kind of finding — a known incident with deliberately
  withheld documentation.

## Process

### F17. "Unsigned commits / no DCO compliance."

Increasingly common finding for software supply chain. Aether's
own contribution policy is DCO sign-off; auditors will check the
deployed version was built from signed-off commits.

- **Aether default**: every Aether commit is DCO-signed. The
  CI gate (`dco` job) refuses unsigned PRs.
- **Operator gotcha**: if you fork and add custom commits, your
  fork must enforce the same. Ship the build provenance
  alongside the audit pack.

### F18. "Unverified container images in production."

- **Aether default**: the release workflow generates an SBOM
  via syft and (planned) signs images with cosign.
- **Operator gotcha**: pulling from `latest` tag is not
  verifiable. Pin to digests in your Helm values and document
  the verification command.

## What this catalogue does NOT cover

- **Jurisdiction-specific findings** (data residency, regulatory
  reporting). Those depend on where you operate.
- **Customer / BSS-side contractual findings**. The auditor
  scopes to what the SAS-SM Standard requires, but your
  customers' contracts may add to the list.
- **Findings tied to platform functionality the project hasn't
  shipped yet** (e.g. SAIP codec, Brainpool curve, multi-region
  active-active). The status table at [index.md](index.md) is
  the source of truth for what's ready.

## Cross-references

- [Gap analysis](gap-analysis.md) — full mapping of SAS-SM
  control families to platform features
- [Key ceremony](key-ceremony.md) — addresses F2 and F3
- [HSM vendor configuration](hsm-vendors.md) — per-vendor `.so`
  paths and quirks for AWS CloudHSM, GCP Cloud HSM, Azure
  Managed HSM, Thales Luna, Utimaco
- [RBAC and segregation of duties](rbac.md) — addresses F4, F10, F11
- [Audit log retention](audit-retention.md) — addresses F4, F5, F6
- [Disaster recovery](disaster-recovery.md) — addresses F14, F15
- [Incident response](incident-response.md) — addresses F16
- [Reference AWS deployment](reference-aws.md) — concrete
  topology referenced throughout
