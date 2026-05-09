# Reference deployment — GCP

A reference topology for an Aether SM-DP+ deployment on Google
Cloud Platform, sized for an MVNO running through SAS-SM
accreditation. Mirrors the structure of
[reference-aws.md](reference-aws.md) so adopters can compare
shapes side by side.

GSMA SAS-SM accreditation specifies the geographic regions where
sensitive processes may run. GCP regions currently qualifying for
GSMA-rated deployments include **europe-west1 (Belgium)**,
**europe-west3 (Frankfurt)**, **us-central1 (Iowa)**, and others.
Confirm the current list with your auditor before you build; the
list does change.

## Topology

```
                          ┌────────────────────────┐
                          │    Public ingress      │
                          │  (Cloud DNS + GCLB +    │
                          │   Cert Manager)         │
                          └───────────┬────────────┘
                                      │ HTTPS + mTLS
                                      ▼
                ┌─────────────────────────────────────┐
                │   GKE cluster — aether namespace    │
                │                                     │
                │   gateway ──┬── smdp-plus           │
                │             ├── smds                │
                │             ├── eim                 │
                │             ├── profile-builder     │
                │             ├── certmgr             │
                │             ├── audit               │
                │             ├── hsm-broker          │
                │             └── admin-ui            │
                └────────────────────┬────────────────┘
                                     │
        ┌────────────────────────────┼────────────────────────────┐
        ▼                            ▼                            ▼
┌──────────────┐            ┌──────────────────┐         ┌────────────────┐
│ Cloud SQL    │            │ Cloud HSM        │         │ GCS bucket     │
│ Postgres 16  │            │ (Marvell LiquidS │         │ with Bucket    │
│ (Regional HA,│            │  ecurity Adapter,│         │ Lock,          │
│  CMEK)       │            │  FIPS 140-2 L3)  │         │ Compliance,    │
└──────────────┘            └──────────────────┘         │ CMEK)          │
                                                          └────────────────┘
```

## Components

### GKE

- One cluster, one namespace per environment (`aether-prod`,
  `aether-staging`).
- **Autopilot** is the recommended starting point for an MVNO
  scale: less node-management overhead, the same security floor
  as Standard GKE. Switch to Standard with a dedicated node pool
  if you need GPU/specific machine types.
- For Standard: `e2-standard-4` minimum, 3 nodes spread across
  zones in the region.
- Pod Security Admission policy: `restricted`. The Helm chart's
  pod security context already meets it.
- Cloud Logging enabled (audit-relevant; the auditor will look
  for it).
- **Workload Identity** binds the per-service `ServiceAccount`s
  the chart creates to GCP IAM service accounts; each service
  gets *only* the permissions it needs (e.g. hsm-broker → Cloud
  HSM client, audit → GCS Object writer for the WORM bucket).

### Cloud SQL

- Postgres 16, regional (multi-zone) HA, encryption at rest
  with a customer-managed encryption key (CMEK) from Cloud KMS.
- Tier: `db-perf-optimized-N-2` minimum (4 vCPU, 16 GB).
- Automated backups: 7-day retention by default; bump to 35-day
  with `--backup-retention-period=35`.
- PITR enabled (Point-In-Time Recovery).
- Audit logging: enable `cloudsql.iam_authentication=on` and
  Cloud SQL audit logs to Cloud Logging.
- Connect via the Cloud SQL Auth Proxy as a sidecar, or via
  Private Service Connect for cleaner networking.
- The Helm chart's `postgres.enabled: false` plus the Cloud SQL
  connection string in `postgresUrl` wires this in.

### Cloud HSM

- Marvell LiquidSecurity adapter behind Cloud HSM. FIPS 140-2
  Level 3 — the SAS-SM Standard's typical baseline.
- HA: spread keys across two HSMs in the region; Cloud HSM
  handles replication. Region selection matters; pick from the
  GSMA-qualifying list above.
- The PKCS#11 client library (`libCryptoki2.so` from the Marvell
  PKCS#11 SDK) is mounted as a volume into the hsm-broker pod.
  The chart's `hsmBroker.backend: external` plus
  `hsmBroker.pkcs11.libraryPath` and `pinSecret.name` wire this
  in.
- The HSM PIN lives in **Google Secret Manager**, mounted as the
  `pin` key of an `aether-hsm-pin` Secret via the Secret Manager
  CSI Driver (or via the `kubernetes-secret-syncer` from
  External Secrets Operator). Custodians rotate via the
  procedure in [key-ceremony.md](key-ceremony.md).

### GCS — audit log offsite

- Bucket with **Bucket Lock** enabled in **retention-policy
  Compliance mode**, default retention 3 years. Like AWS S3
  Object Lock Compliance, even the bucket owner cannot shorten
  retention once locked.
- CMEK encryption with a customer-managed key that the audit
  service account (and only it) can use to write.
- **Dual-region** bucket type for in-region redundancy plus
  reach to a second qualifying region; alternatively,
  cross-region replication via a second bucket with a
  Storage Transfer Service.
- Lifecycle rule transitions objects to Coldline after 90 days
  (cold-storage tiering — operator decision).

### Networking

- Private GKE cluster (no public node IPs); control plane
  reachable via authorised networks only.
- **External HTTPS Load Balancer** terminates HTTPS and mTLS
  for ES2+. Mutual TLS configured via a per-LB
  `ServerTlsPolicy` resource.
- VPC firewall rules deny egress except: `private.googleapis.com`
  (Google APIs over private service connect), Cloud SQL via
  PSC, Cloud HSM endpoints, the WORM GCS bucket via Private
  Google Access, and your CI's OCSP responder.
- VPC Flow Logs enabled. The auditor will look for them.

## Deployment

The `deployments/helm/aether/` chart drives the install. Sample
production override (file the operator owns, kept out of this
repo):

```yaml
postgres:
  enabled: false
postgresUrl: "postgres://aether:$PG_PASSWORD@127.0.0.1:5432/aether?sslmode=require"
# Cloud SQL Auth Proxy runs as a sidecar in each pod; the
# proxy listens on 127.0.0.1:5432 inside each pod.

hsmBroker:
  backend: external
  pkcs11:
    libraryPath: /opt/marvell/lib/libCryptoki2.so
    slot: "0"
    pinSecret:
      name: aether-hsm-pin   # populated via Secret Manager CSI

certmgr:
  mode: production
  trustStore: |
    -----BEGIN CERTIFICATE-----
    ...your GSMA CI roots here...
    -----END CERTIFICATE-----
  identitySecret:
    name: aether-identity-keys

ingress:
  enabled: true
  className: gce
  annotations:
    networking.gke.io/managed-certificates: aether-mc-cert
    kubernetes.io/ingress.class: gce
    kubernetes.io/ingress.allow-http: "false"
  host: rsp.your-mvno.com
  tls:
    enabled: true
    secretName: aether-tls

global:
  imageRegistry: us-central1-docker.pkg.dev/your-gcp-project/aether

metrics:
  prometheusAnnotations: true

observability:
  prometheusOperator:
    enabled: true   # if you've installed kube-prometheus-stack
```

## Observability

- Google Cloud Operations (formerly Stackdriver) is the
  default — Cloud Monitoring scrapes Prometheus annotations,
  Cloud Logging absorbs structured JSON.
- Alternatively run kube-prometheus-stack in-cluster and apply
  the bundled rules from
  [deployments/observability/](https://github.com/ajamous/aether/tree/main/deployments/observability).
- Cloud Monitoring alerting policies for the four must-haves
  (see [reference-aws.md](reference-aws.md) §"Observability"
  for the same list adapted to CloudWatch); replace each with
  the equivalent Cloud Monitoring policy.
- Cloud Logging Insights queries pre-baked for the audit
  evidence pack:
  - "All Sign calls in the last 30 days, by key id"
  - "All HTTP 401 from authenticateClient, by source IP"
  - "All cert rotations, by operator"

## Backup and disaster recovery

- Cloud SQL automated backups: 35-day retention.
- Manual on-demand backup before every Aether minor-version
  upgrade.
- Audit log offsite: nightly dump → WORM GCS bucket; the bucket's
  dual-region type provides in-region redundancy automatically.
- Cross-region: Storage Transfer Service replicates to a second
  qualifying region.
- Tested annually: full restore in a separate VPC, run
  `/v1/verify`, confirm `ok=true` against the production tail
  hash recorded in the daily timeline anchor (see
  [audit-retention.md](audit-retention.md)).

See [disaster-recovery.md](disaster-recovery.md) for the
recovery procedure across single-zone, regional, and database-
compromise scenarios.

## Cost ballpark

For an MVNO at the lower end of the SM-DP+ traffic profile
(US-pricing rough numbers; GCP committed-use discounts can shave
significantly):

- GKE Autopilot baseline: ~$75/mo control plane + workload pricing
- Cloud SQL Enterprise Plus 4 vCPU 16GB regional HA: ~$450/mo
- Cloud HSM (2 partitions for HA): ~$2,000/mo  ← dominant cost
- GCS dual-region + Coldline transitions: <$60/mo for normal traffic
- External HTTPS LB: ~$25/mo + traffic
- Cloud Logging / Cloud Monitoring: low-volume, mostly free tier

Cloud HSM is the dominant cost, as on AWS. Operators with
on-prem HSMs already in service can skip this and follow
[reference-onprem.md](reference-onprem.md) instead; the
hsm-broker is identical.

## What this reference does NOT include

- Multi-region active-active. Single region is correct for
  SAS-SM baseline; multi-region is a Phase 6 follow-up.
- Cost optimisation past the basic shape.
- The Cloud HSM key ceremony. Terraform creates the key ring
  and IAM bindings; the two-person key-ceremony procedure in
  [key-ceremony.md](key-ceremony.md) generates the actual
  identity keys against it.
- Workload Identity binding to chart ServiceAccounts. The
  Terraform module creates the audit and hsm-broker GCP service
  accounts but the binding to the chart's Kubernetes
  ServiceAccount (named `<release>-aether`) is documented as a
  post-deploy `gcloud iam service-accounts add-iam-policy-binding`
  call. See the module's README.

The Terraform modules implementing this topology live under
`deployments/terraform/gcp/`. See that directory's README for
inputs, outputs, and the `examples/full` canonical wiring.

## Cross-references

- [Reference AWS deployment](reference-aws.md) — same shape on AWS
- [Reference Azure deployment](reference-azure.md) — same shape on Azure
- [Reference on-prem deployment](reference-onprem.md) — same shape on-prem
- [Helm chart](https://github.com/ajamous/aether/tree/main/deployments/helm/aether)
- [Gap analysis](gap-analysis.md) — what each component satisfies
- [Key ceremony](key-ceremony.md) — runs against the Cloud HSM
  cluster
- [Audit retention](audit-retention.md) — WORM-bucket details
