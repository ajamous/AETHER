# SAS-SM readiness

Aether cannot be SAS-SM accredited. Only deployments are. What we can
do — and what this section is — is make accreditation as cheap as
possible for the MVNOs and small carriers who choose to pursue it.

Everything in this section is **free, in-repo, no gatekeeping**. There
is no Enterprise tier where the audit-ready templates live. The
moment a small MVNO walks into an audit using these and passes, that
becomes the most powerful organic signal this project will ever
produce. We do not reserve that win for a paid edition.

## What lives here

When this section is fleshed out (Phase 6+), expect:

| Document                                                     | Status   |
| ------------------------------------------------------------ | -------- |
| Gap analysis: SAS-SM Standard requirement → Aether feature   | Planned  |
| Reference deployment topology: AWS GSMA-certified region     | Planned  |
| Reference deployment topology: GCP                           | Planned  |
| Reference deployment topology: on-prem (Thales / Utimaco)    | Planned  |
| HSM key ceremony script with chain-of-custody form templates | Planned  |
| Logging and audit retention defaults (3 years, immutable)    | Planned  |
| RBAC and segregation-of-duties templates                     | Planned  |
| Incident response runbook with severity matrix               | Planned  |
| Annual recertification checklist                             | Planned  |
| Common audit findings and Aether's defaults that pre-empt them | Planned |
| Worked examples of evidence packages from real audits        | Planned  |

The "worked examples" come from adopters who pass an audit and
contribute back anonymized lessons learned. This is invitation, not
requirement — but it is the dynamic that makes this section get
better over time.

## What an MVNO should expect

The goal is that a deployer using Aether can walk into an NCC Group
SAS-SM audit with **70%+ of the evidence already produced by the
platform itself**, and the remaining 30% guided by templates rather
than guessed at from scratch.

That is the bar. We are not there yet. We will tell you honestly
when we are.

## What we cannot do

- We cannot make your deployment SAS-SM accredited. Auditors do that.
- We cannot answer auditor questions for you. The templates here
  give you the structure; the answers are yours.
- We cannot guarantee any specific audit will pass. Compliance
  outcomes depend on operator practice as much as platform features.

## How to contribute back

If you deploy Aether and pass a SAS-SM audit:

1. Anonymize whatever you contribute (no carrier names, no PII)
2. Open a PR adding a worked example to `docs/sas-sm/evidence/`
3. We will review for accuracy and merge

This is how the section gets better: adopters teaching the next
adopter. Reciprocity, not transaction.
