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
| `pkg/saip`                    | Partial       | SGP.22 §B SAIP codec: ProfileHeader + PE-USIM (IMSI/PLMN) + PE-AKAParameter (Milenage Ki/OPc) + PEEnd, DER round-trip, AppendRaw for spare ProfileElements. Further ProfileElements (PE-PinCodes, PE-FileSystem, …) and per-element TCA §B wire framing (EF packing, PE-Header wrappers) land as the catalogue grows |
| `services/hsm-broker`         | Implemented   | Memory + SoftHSM backends with real ECDSA Sign / ECKA Derive / GenerateKeyPair / ListKeys verified against SoftHSM v2; cloud HSM backends pending |
| `services/certmgr`            | Implemented   | Cert chain load/verify, lab/prod modes, expiry metrics, lab-chain generator |
| `services/smdp-plus`          | Partial       | ES9+ endpoints + persistent sessions + signed `ServerSigned1` (§5.7.13) + verified `EuiccSigned1` (§5.7.5) + spec-faithful `AuthenticateServerResponse` outer SEQUENCE (Annex B) + DPpb-signed `SmdpSigned2` (§5.7.14) + signature-verified `PrepareDownloadResponse` (§5.7.7) + BPP session-key derivation (§H.3 ECKA → SCP03t SENC/SMAC/MCV) + AES-128-GCM segment seal/open with MAC chaining + outer `BoundProfilePackage` (§5.7.6) assembly **and** disassembly with `InitialiseSecureChannelRequest` (§5.7.7) — every BPP layer codec ships in `internal/bpp` and the `getBoundProfilePackage` handler returns a real DER-encoded BPP when DPpb is configured (lab mode without DPpb still returns honest 501). `POST /v1/profiles/prepare` (in-tree stand-in for ES2+ DownloadOrder) builds a credential-carrying UPP via profile-builder, mints a matchingId + SGP.22 §4.1 activation code, and the eUICC's later download resolves it by matchingId or ICCID. Server tests decrypt the BPP and confirm the operator's IMSI/Ki/OPc round-trip. Spec-precise per-segment AAD layout remains a hardware-bench follow-up |
| `services/smds`               | Partial       | ES11 + ES12 with end-to-end discovery flow, in-memory and Postgres-backed event stores, SGP.22 §5.5.4 ServerSigned1 ECDSA-SHA-256 signing via hsm-broker (opt-in via `--hsm-broker`). LPA-side verification against an SM-DS identity certificate is a follow-up |
| `services/eim`                | Skeleton      | SGP.32 device registry + per-device command queue, in-memory and Postgres-backed; IPA poll/ack lifecycle tested end to end |
| `services/profile-builder`    | Partial       | YAML template loader + UPP envelope + real DER-encoded SAIP via `pkg/saip` carrying the subscriber's IMSI/PLMN (PE-USIM) and Milenage Ki/OPc (PE-AKAParameter); richer ProfileElements land as `pkg/saip` grows |
| `services/audit`              | Implemented   | Hash-chained ledger with verify, in-memory and Postgres-backed stores; serializable concurrent appends verified; signed timeline anchors at `/v1/anchor` (ECDSA-SHA-256 over DER-encoded `(timestamp, length, tail_hash)` SEQUENCE; opt-in via `--hsm-broker`); offline auditor CLI under `tools/aether-verify-anchor/` (`make verify-anchor`) |
| `services/gateway`            | Implemented   | ES2+ shapes + REST proxy + HTTPS + verified-client-cert mTLS on /gsma/rsp2/es2plus/* (path-scoped) + per-source-IP token-bucket rate limiter on /gsma/rsp2/* + Bearer-token OIDC on /v1/* admin paths (RS256 + ES256, JWKS cache, /v1/health and /metrics bypass). ES2+ DownloadOrder forwards to smdp-plus `/v1/profiles/prepare` when the order carries subscriber data. All security flags opt-in; lab default disabled |
| `ui/admin`                    | Partial       | Next.js 15 read-only console with Auth.js OIDC sign-in (lab bypass with banner when unconfigured); dashboard, templates, certs, SM-DS, eIM, audit |
| Lab Docker Compose            | Implemented   | `make lab-up` brings up the full stack; smoke tests under `test/e2e` |
| Conformance harness (SGP.23)  | Implemented   | `make conformance` runs 92 cases across 10 families; coverage matrix in `tools/conformance/coverage/sgp23.md`; hardware-in-the-loop tests honestly out of scope |
| Cloud HSM backends            | Implemented (PKCS#11) | One PKCS#11 backend exercised end-to-end against SoftHSM v2 in CI; `docs/sas-sm/hsm-vendors.md` documents the per-vendor plumbing for AWS CloudHSM, GCP Cloud HSM (KMS PKCS#11), Azure Managed HSM, Thales Luna, and Utimaco SecurityServer. Per-vendor hardware-in-the-loop verification stays an honest follow-up bench |
| Helm chart                    | Implemented   | Lab + production install paths; lab cert-init initContainer auto-mints SGP.26-style chain on every pod start |
| Terraform modules             | Implemented   | AWS (`deployments/terraform/aws/`: VPC + EKS + RDS Multi-AZ + CloudHSM + WORM S3), GCP (`deployments/terraform/gcp/`: VPC + GKE Autopilot + Cloud SQL regional HA + Cloud HSM key ring + Bucket-Locked GCS), and Azure (`deployments/terraform/azure/`: VNet + AKS private + Postgres Flexible zone-redundant + Managed HSM FIPS 140-3 L3 + immutable Storage GZRS) reference deployments. IRSA / Workload Identity / federated-credential binding is a documented manual post-deploy step on each |
| Observability bundle          | Implemented   | 12 Prometheus alert rules (cert expiry, audit chain integrity, service health, HSM ready + p99 latency, ES2+ 401 spike per reason, gateway rate-limit, Postgres), ServiceMonitor manifests, and four Grafana dashboards (overview, HSM, gateway auth gates, audit chain) under `deployments/observability/` |
| SAS-SM evidence templates     | Implemented   | Gap analysis, key ceremony, HSM vendor configuration, RBAC, audit retention, AWS + GCP + Azure + on-prem reference deployments, DR runbook, incident response, common audit findings, recertification checklist, release verification (cosign + Sigstore) — all in `docs/sas-sm/` (free, no paid tier) |

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
