# SAS-SM readiness

This section is for MVNOs and small carriers preparing for a SAS-SM
(Security Accreditation Scheme for Subscription Management) audit.
Aether cannot be SAS-SM accredited — only deployments are. What this
section does is **make accreditation as cheap as possible** for the
adopters who choose to pursue it.

Everything here is **free, in-repo, no gatekeeping**. There is no
Enterprise tier where the audit-ready templates live. The first MVNO
that walks into an NCC Group audit with these templates and passes
becomes the most powerful organic signal this project will ever
produce. We do not reserve that win for a paid edition. See
[GOVERNANCE.md](https://github.com/ajamous/aether/blob/main/GOVERNANCE.md) §"What this project commits to"
for the binding statement.

## Status

| Document                                                         | Status        |
| ---------------------------------------------------------------- | ------------- |
| [Gap analysis: SAS-SM requirements → Aether features](gap-analysis.md) | Implemented   |
| [Key ceremony procedure + chain-of-custody form](key-ceremony.md)      | Implemented   |
| [RBAC + segregation-of-duties templates](rbac.md)                      | Implemented   |
| [Audit log retention defaults](audit-retention.md)                     | Implemented   |
| [Reference deployment topology — AWS GSMA-certified regions](reference-aws.md) | Implemented   |
| [Reference deployment topology — GCP](reference-gcp.md)                | Implemented   |
| [Reference deployment topology — on-prem (Thales / Utimaco)](reference-onprem.md) | Implemented   |
| [Common audit findings and how Aether's defaults pre-empt them](common-findings.md) | Implemented   |
| [Annual recertification checklist](recertification-checklist.md)       | Implemented   |
| [Disaster recovery runbook](disaster-recovery.md)                      | Implemented   |
| [Incident response runbook with severity matrix](incident-response.md) | Implemented   |
| Worked examples of evidence packages from real audits                  | Not started — contributed by adopters who pass |

## How to use this section

If you are an MVNO planning a SAS-SM audit:

1. Read [gap-analysis.md](gap-analysis.md). It maps every SAS-SM
   Standard requirement Aether currently addresses to the specific
   feature, configuration, or control that satisfies it. For
   requirements Aether does not satisfy, it tells you what you have
   to add operationally.
2. Run a [key ceremony](key-ceremony.md) before you load production
   keys into your HSM. The chain-of-custody form goes into the
   evidence pack you hand the auditor.
3. Apply the [RBAC template](rbac.md) to your Kubernetes cluster.
   The auditor will look for segregation of duties; the template
   gives you that out of the box.
4. Confirm your [audit log retention](audit-retention.md) defaults.
   Aether's append-only hash-chained log carries most of the
   technical work; this doc tells you what to add operationally.
5. Stand up your environment using [reference-aws.md](reference-aws.md)
   (or the equivalent on GCP / on-prem when those land).

The bar is **70%+ of the evidence already produced by the platform
itself, and the remaining 30% guided by templates rather than
guessed at from scratch**. We are not there yet — the table above
is honest about what's missing — but each item that lands moves an
adopter closer to that bar.

## What we cannot do

- We cannot make your deployment SAS-SM accredited. Auditors do
  that.
- We cannot answer auditor questions on your behalf. The templates
  give you structure; the answers are yours.
- We cannot guarantee any specific audit will pass. Compliance
  outcomes depend on operator practice as much as platform features.

## How to contribute back

If you deploy Aether and pass a SAS-SM audit:

1. Anonymize whatever you contribute — no carrier names, no PII.
2. Open a PR adding a worked example to `docs/sas-sm/evidence/`.
3. We'll review for accuracy and merge.

This is how the section gets better: adopters teaching the next
adopter. Reciprocity, not transaction.

## Cross-references

- [ADR 0003 — PKCS#11 HSM abstraction](../architecture/adr/0003-pkcs11-abstraction.md)
- [ADR 0004 — Lab vs production cert mode](../architecture/adr/0004-lab-vs-prod-cert-mode.md)
- [services/audit](https://github.com/ajamous/aether/tree/main/services/audit) — the hash-chained
  ledger that backs most of the audit-trail evidence
- [services/certmgr](https://github.com/ajamous/aether/tree/main/services/certmgr) — the cert
  custody surface
- [services/hsm-broker](https://github.com/ajamous/aether/tree/main/services/hsm-broker) — the
  PKCS#11 façade that gives you cloud-HSM portability
