# SAS-SM gap analysis

Map of SAS-SM Standard control families to what Aether already
provides, what the operator must add, and what the auditor will
expect to see as evidence. Use this as the spine of your audit
preparation.

The control-family numbering follows the GSMA SAS-SM Standard's
common organisational pattern (security policy, sensitive process,
key management, network, audit, incident, etc.). The exact section
numbers in your audit will track the specific Standard version your
auditor is working from; the structure below is stable across
versions.

> **Honest note**: this gap analysis is a starting point, not an
> auditor-blessed checklist. Adopters who go through an audit are
> encouraged to contribute back the version that actually held up
> in the room (anonymised). See `docs/sas-sm/index.md` §"How to
> contribute back".

## How to read this table

- **Aether feature** is what already exists in this repo.
- **Operator-supplied control** is what you, the deployer, must
  add. These are the items the platform cannot do for you.
- **Evidence** is what to put in your audit pack. "Aether-emitted"
  means the platform produces it automatically.

## A. Security Policy and Organisation

| Requirement family | Aether feature | Operator-supplied control | Evidence |
| --- | --- | --- | --- |
| Documented security policy | None — your policy is yours | A written policy aligned with SAS-SM Standard §A | Policy document, version history |
| Roles and responsibilities | RBAC template ([rbac.md](rbac.md)) | Map your team to the template's roles; document approvals | RBAC manifests applied to cluster, role assignment records |
| Risk management | Threat-model document under `docs/architecture/threat-model.md` (planned) | Risk register | Risk register with owners and review dates |
| Background checks for sensitive-role staff | Out of scope | HR / vetting process | HR records (anonymised) |

## B. Sensitive Process — Profile preparation and delivery

| Requirement | Aether feature | Operator-supplied control | Evidence |
| --- | --- | --- | --- |
| ES2+ DownloadOrder authenticated | `services/gateway` enforces verified client cert on `/gsma/rsp2/es2plus/*` against a configured CA bundle (path-scoped: `/v1/*` admin paths unaffected) | BSS-side cert ceremony; trust-store CA management | Gateway TLS + ES2+ CA Secret manifests, BSS client cert chain, audit log entries showing 401/200 by source |
| Profile package generation in trusted zone | `services/profile-builder` runs in-cluster, no internet egress | Network policy isolating profile-builder | NetworkPolicy YAML, cluster ingress logs |
| Profile keys (Ki, OPc) never appear in logs | `services/profile-builder` validates inputs but does not log them; structured logging strips byte fields | Log-pipeline policy (no debug-level logs in prod) | Sample log dump showing redaction; logging config |
| Profile delivery cryptographically bound to target eUICC | `pkg/crypto/bsp` (AES-128-GCM) + ECKA via `pkg/crypto/ecka` and `services/hsm-broker` | None — platform-provided | BPP traces from `services/audit` |
| ES9+ ServerSigned1 produced under HSM control | `services/smdp-plus` signs via `services/hsm-broker`; private key never leaves HSM | HSM hardening (FIPS-rated module + lifecycle) | HSM model + FIPS cert, hsm-broker config, audit entries showing Sign calls |
| eUICC chain validated on authenticateClient | `services/smdp-plus` verifies leaf → EUM → CI root using `services/certmgr` trust store | Operator chooses CI roots accepted | certmgr trust-store config, audit entries showing 401 rejections of bad chains |
| Replay defense on auth handshake | serverChallenge bound into both ServerSigned1 and EuiccSigned1 (verified) | None | Audit entries showing serverChallenge values per session |

## C. Key Management

| Requirement | Aether feature | Operator-supplied control | Evidence |
| --- | --- | --- | --- |
| Production private keys stored in FIPS-rated HSM | PKCS#11 abstraction supports SoftHSM (lab), Thales Luna, Utimaco, AWS CloudHSM, GCP/Azure Managed HSM | Choose a FIPS-140-2 / 140-3 rated HSM in production | HSM model + FIPS certification PDF |
| Keys generated inside HSM | hsm-broker's GenerateKeyPair calls `CKM_EC_KEY_PAIR_GEN` against the configured PKCS#11 module | None — platform-provided | hsm-broker startup logs, key ceremony minutes ([key-ceremony.md](key-ceremony.md)) |
| No private-key material crosses the broker boundary | hsm-broker has no `Export` operation. ECKA result lives as a handle. Documented in [ADR 0003](../architecture/adr/0003-pkcs11-abstraction.md) | None | ADR 0003 + hsm-broker code review |
| Key ceremony with chain of custody | [key-ceremony.md](key-ceremony.md) — concrete procedure + form | Run the ceremony with the prescribed quorum | Signed chain-of-custody form (copy in audit pack) |
| Key rotation procedure | certmgr rotation API (skeleton) + key-ceremony procedure for new keys | Schedule + execute rotations per policy | Rotation log, ceremony forms for rotation events |
| Trust store integrity (CI roots) | `services/certmgr` loads from configured PEM; `/v1/trust-store/pem` lets services fetch | Pin CI roots in a versioned ConfigMap; review on each cert update | Versioned trust-store ConfigMap, change-control record |
| Key expiry monitoring and alerting | `aether_cert_expiry_days` Prometheus metric per cert | Alertmanager rules; on-call rota | Alert rule YAML, on-call runbook |

## D. Network and Infrastructure Security

| Requirement | Aether feature | Operator-supplied control | Evidence |
| --- | --- | --- | --- |
| TLS for all external interfaces | Gateway terminates HTTPS; smdp-plus accepts mTLS | Operator-supplied certs and ingress | Gateway TLS config, ingress manifests |
| mTLS for ES2+ inbound | Gateway listener mTLS with `VerifyClientCertIfGiven` plus per-request middleware that rejects unauthenticated requests on `/gsma/rsp2/es2plus/*` (401) and lets `/v1/*` through unchanged. Verified by integration tests in `services/gateway/internal/server`. | BSS client cert chain provisioning + Secret population | Helm-rendered `aether-tls` and `aether-bss-ca` Secrets, gateway args showing the TLS flags, sample 401 from a request without a client cert |
| Service-to-service trust | Cluster-internal only by default; service mesh ready | mTLS via mesh (Linkerd, Istio) or NetworkPolicy isolation | Mesh config or NetworkPolicy YAML |
| Datastore encryption at rest | Postgres supports filesystem encryption; bring your own | Operator picks RDS encryption / disk encryption | Cloud config showing encryption-at-rest enabled |
| Network segmentation | Helm chart deploys all services in one namespace by default | NetworkPolicy per service tier | NetworkPolicy YAML |
| Hardened pod runtime | Helm chart's pod security context: runAsNonRoot, runAsUser=65532, readOnlyRootFilesystem, drop ALL capabilities | None — platform-provided | `helm template` output of running release |

## E. Audit and Logging

| Requirement | Aether feature | Operator-supplied control | Evidence |
| --- | --- | --- | --- |
| Append-only audit log of sensitive operations | `services/audit` hash-chained Postgres ledger ([audit-retention.md](audit-retention.md)) | DB user GRANTs revoke UPDATE/DELETE on `audit_entries` | Postgres role grants, sample chain integrity check from `/v1/verify` |
| Tamper detection | SHA-256 chain over (seq, ts, payload, prev_hash) | Periodic external verify | `/v1/verify` cronjob + alert on `ok=false` |
| Per-event integrity | Each `audit_entries` row carries its own hash, plus the chain link | None | Sample row export showing all five fields |
| Log retention period | Configurable; default 3 years immutable in [audit-retention.md](audit-retention.md) | Backup + offsite copy policy | Retention policy doc, backup verification |
| Log review process | None — operational | Documented review cadence | Review log entries with reviewer name |

## F. Personnel and Access

| Requirement | Aether feature | Operator-supplied control | Evidence |
| --- | --- | --- | --- |
| Segregation of duties | RBAC roles in [rbac.md](rbac.md): `aether-operator`, `aether-auditor`, `aether-key-custodian`, `aether-incident-responder` | Map roles to humans; no one holds all four | RBAC manifests, role-assignment record |
| Privileged access review | RBAC manifests stored in Git; admin UI sign-in via Auth.js OIDC delegates identity to operator's IdP, so user provisioning / off-boarding rides existing controls | Quarterly review with approver sign-off; IdP group → role mapping documented | Git history of RBAC changes, review minutes, IdP group export |
| Least privilege for service accounts | Helm chart creates per-release SA, no cluster-admin | Operator audits cluster-wide SA grants | `kubectl auth can-i --list --as system:serviceaccount:NS:aether` output |
| Multi-person key custody | [key-ceremony.md](key-ceremony.md) requires two-person quorum | Operator runs the ceremony | Signed ceremony forms |
| Termination procedure | Out of scope | HR off-boarding tied to RBAC removal | Off-boarding checklist, evidence of timely revocation |

## G. Incident Management

| Requirement | Aether feature | Operator-supplied control | Evidence |
| --- | --- | --- | --- |
| Incident response procedure | [incident-response.md](incident-response.md) — severity matrix (Sev-1 covers audit-chain breach), audit-chain-break sub-procedure, postmortem template | On-call rota, escalation contacts, IdP wiring | Runbook published, last incident's postmortem |
| Postmortem requirement | [incident-response.md](incident-response.md) §"Postmortem" prescribes the format; postmortems land under `docs/operations/postmortems/` | Internal review of every Sev-1/2 | Published postmortem (or attestation of "no Sev-1/2 incidents") |
| Vulnerability disclosure | [SECURITY.md](https://github.com/ajamous/aether/blob/main/SECURITY.md) — 90-day default disclosure window | Designate a security contact | SECURITY.md, mailing list / advisory channel |
| Patch management | Quarterly minor releases per [GOVERNANCE.md](https://github.com/ajamous/aether/blob/main/GOVERNANCE.md) | Operator-driven patch SLA | Change log, patch-application records |

## H. Business Continuity

| Requirement | Aether feature | Operator-supplied control | Evidence |
| --- | --- | --- | --- |
| Backup and recovery | Postgres-backed state in audit, smds, smdp-plus, eim | Operator-managed backups (RDS snapshots, etc.) | Backup-restore drill record |
| Disaster recovery plan | [disaster-recovery.md](disaster-recovery.md) covers single-AZ failure (transparent), regional outage (cold-start in qualifying secondary region), and database compromise (audit-chain-break sub-procedure). Multi-region active-active still Phase 6 | RTO/RPO sign-off, drill cadence, DNS / traffic-shaping migration plan | DR runbook published, last drill report, /v1/verify output post-restore |
| RTO / RPO documentation | None — operational | Documented per service | RTO/RPO table reviewed annually |

## What still has gaps

The "Operator-supplied control" column above is honest about what
the platform cannot do for you. The biggest current gaps the
platform plans to close:

1. **Multi-region active-active reference deployment** — Phase 6
   of the project plan. The DR runbook covers cold-start to a
   secondary region today; active-active tightens RPO from
   24 hours to minutes for the audit chain.
2. **Bundled Grafana dashboards / Alertmanager rules** for the
   operational observability requirements — coming with the
   observability work, see Phase 0/1 follow-ups.
3. **GCP and on-prem reference deployments** — only AWS today
   (see [reference-aws.md](reference-aws.md)).
4. **Worked evidence package examples** — pending adopters passing
   audits and contributing back ([common-findings.md](common-findings.md)
   describes the patterns; the worked examples close the loop).
