# Aether

Open Source Remote SIM Provisioning for the Open Telecom Era.

A GSMA-compliant SM-DP+, SM-DS, and eIM stack that runs on a laptop today and
graduates to production the day a carrier swaps in their GSMA-rooted
certificates. Same binary, same architecture, same UI from lab to scale.

```
git clone https://github.com/ajamous/aether
cd aether
make lab-up
```

That's the goal: less than 60 seconds from clone to a running stack with
SGP.26 test certs and a SoftHSM-backed key store, ready to talk to a real
sysmoEUICC test card. We are not there yet — see Status below.

## Status

This project is in **Phase 0** (foundation). Nothing here is production-ready.
The table below is the source of truth.

| Component                     | Status        | Notes                                              |
| ----------------------------- | ------------- | -------------------------------------------------- |
| Repo bootstrap                | Implemented   | License, governance, CI, Makefile, lint configs    |
| Documentation skeleton        | Implemented   | MkDocs Material site, ADRs 0001-0005               |
| ASN.1 toolchain               | Skeleton      | Build glue + starter type with round-trip tests; spec modules pending vendoring |
| `pkg/crypto`                  | Implemented   | ECDSA P-256, ECKA, X9.63-SHA-256 KDF, AES-128-GCM BSP. Brainpool P-256 r1 stubbed |
| `services/hsm-broker`         | Implemented   | Memory + SoftHSM backends with real ECDSA Sign / ECKA Derive / GenerateKeyPair / ListKeys verified against SoftHSM v2; cloud HSM backends pending |
| `services/certmgr`            | Implemented   | Cert chain load/verify, lab/prod modes, expiry metrics, lab-chain generator |
| `services/smdp-plus`          | Partial       | ES9+ endpoints + persistent sessions + signed `ServerSigned1` (SGP.22 §5.7.13) via hsm-broker; BPP returns 501 until SAIP codec lands |
| `services/smds`               | Skeleton      | ES11 + ES12 with end-to-end discovery flow, in-memory and Postgres-backed event stores; signing pending |
| `services/eim`                | Not started   | SGP.32                                             |
| `services/profile-builder`    | Skeleton      | YAML template loader + UPP envelope; SAIP-encoded UPP pending |
| `services/audit`              | Implemented   | Hash-chained ledger with verify, in-memory and Postgres-backed stores; serializable concurrent appends verified |
| `services/gateway`            | Skeleton      | ES2+ shapes + REST proxy to upstream services; OIDC/mTLS/rate-limit pending |
| `ui/admin`                    | Skeleton      | Next.js 15 read-only console: dashboard, templates, certs, audit |
| Lab Docker Compose            | Implemented   | `make lab-up` brings up the full stack; smoke tests under `test/e2e` |
| Conformance harness (SGP.23)  | Not started   |                                                    |
| Cloud HSM backends            | Not started   | AWS, GCP, Azure, Thales Luna, Utimaco              |
| Helm chart                    | Implemented   | Lab + production install paths render and lint; cert-init Job for lab pending |
| Terraform modules             | Not started   |                                                    |
| SAS-SM evidence templates     | Not started   | Free in-repo, no paid tier — see `docs/sas-sm/`    |

"Skeleton" means the service runs, exposes the documented HTTP
shape, has tests passing, and has the dependencies it needs for
forward work — but does not yet do the cryptographically-correct
RSP protocol work that would let it talk to a real eUICC. The
README of each service spells out exactly what's implemented vs
pending. We refuse to mark anything Implemented based on shape
alone.

Anything you don't see in the table doesn't exist yet. We will not call
anything "production-ready" until at least one external party runs it in
production and reports back.

## What this is

Aether implements the GSMA Remote SIM Provisioning specs end to end: SGP.22
(Consumer) and SGP.32 (IoT). It is designed to do two things equally well:

- Run on a laptop with sysmoEUICC test cards, for engineers learning RSP
- Run in a SAS-SM-accredited deployment with on-prem or cloud HSMs, for
  small carriers and MVNOs that don't want to pay seven figures for a
  closed-source vendor stack

The same binary does both. The only thing that changes between modes is the
config file and the certificate set.

## What this is not

- Not an HSS/AuC. We integrate with existing core network elements via
  standard interfaces.
- Not a GSMA Certificate Issuer. We consume CI certs; we don't issue them.
- Not a billing or BSS. We expose ES2+ and let the carrier's BSS drive.
- Not a legacy SGP.01/02 (M2M) implementation. SGP.32 supersedes it.
- Not a commercial product. There is no Enterprise edition, no Cloud
  edition, no paid support tier. See `GOVERNANCE.md` for the full
  commitments.

## Standards we implement

| Spec     | Target version              | Coverage                               |
| -------- | --------------------------- | -------------------------------------- |
| SGP.21   | v3.x (Consumer Architecture) | Architectural alignment               |
| SGP.22   | v3.x (Consumer Technical)    | Full RSP protocol stack                |
| SGP.23   | latest (Conformance)         | Test harness integration               |
| SGP.24   | latest (Compliance Process)  | Audit-trail and evidence templates     |
| SGP.26   | latest (Test Certificates)   | Default lab mode                       |
| SGP.31   | latest (IoT Architecture)    | Architectural alignment                |
| SGP.32   | v1.x (IoT Technical)         | eIM, IPA-server, SM-DP+ IoT extensions |
| TCA SAIP | v2.x                         | Profile package generation             |

Every spec-implementing function in the codebase carries a doc comment with
its section reference (e.g. `// Implements SGP.22 §5.7.5 AuthenticateClient`).
That isn't compliance hygiene — it's so the codebase reads as a free RSP
textbook for anyone trying to learn how this works.

## Quick links

- [Plan and roadmap](ROADMAP.md)
- [Contributing](CONTRIBUTING.md)
- [Governance and the no-commercial-entity commitment](GOVERNANCE.md)
- [Security policy](SECURITY.md)
- [Code of conduct](CODE_OF_CONDUCT.md)
- [Maintainers](MAINTAINERS.md)
- [Changelog](CHANGELOG.md)

## License

Apache 2.0. See [LICENSE](LICENSE).

Contributions accepted under the
[Developer Certificate of Origin](https://developercertificate.org/) — sign
your commits with `git commit -s`. There is no CLA.
