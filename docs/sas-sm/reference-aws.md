# Reference deployment — AWS

A reference topology for an Aether SM-DP+ deployment on AWS, sized
for an MVNO running through SAS-SM accreditation. This is one
known-working shape; it is not the only correct shape.

GSMA SAS-SM accreditation specifies the geographic regions where
Aether's sensitive processes may run. AWS regions currently
qualifying include **us-east-2 (Ohio)**, **eu-west-3 (Paris)**,
and a handful of others. Confirm the current list with your
auditor before you build; the list does change.

## Topology

```
                          ┌────────────────────────┐
                          │    Public ingress      │
                          │    (Route 53 + ALB)    │
                          └───────────┬────────────┘
                                      │ HTTPS + mTLS
                                      ▼
                ┌─────────────────────────────────────┐
                │   EKS cluster — aether namespace    │
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
│ AWS RDS for  │            │ AWS CloudHSM     │         │ S3 (Object     │
│ Postgres 16  │            │ cluster          │         │  Lock,         │
│ (Multi-AZ,   │            │ (single-region   │         │  Compliance    │
│  encrypted)  │            │  HA, FIPS 140-2) │         │  mode, KMS-CMK)│
└──────────────┘            └──────────────────┘         └────────────────┘
```

## Components

### EKS

- One cluster, one namespace per environment (`aether-prod`,
  `aether-staging`).
- Node group: m6i.xlarge or larger, 3 nodes minimum, spread across
  AZs.
- Pod security standards: `restricted`. The Helm chart's pod
  security context already meets this.
- Cluster logging enabled to CloudWatch (audit-relevant; auditor
  will look for it).
- IRSA (IAM Roles for Service Accounts) bound to the per-service
  `ServiceAccount`s the chart creates; each service gets *only*
  the AWS permissions it needs (e.g. hsm-broker → CloudHSM client,
  audit → S3 PutObject for the WORM bucket).

### RDS

- Postgres 16, Multi-AZ, encryption at rest with a customer-managed
  KMS key.
- Instance: db.m6g.large minimum.
- Backup retention: 35 days, automated.
- Audit-relevant: Performance Insights and PG audit logging both
  on, exporting to CloudWatch Logs.
- The Helm chart's `postgres.enabled: false` plus the RDS
  connection string in `postgresUrl` wires this in.

### CloudHSM

- One cluster, two HSMs minimum for HA (cross-AZ).
- FIPS 140-2 Level 3 — the SAS-SM Standard's typical baseline.
- The PKCS#11 client library is mounted as a volume into the
  hsm-broker pod; the chart's `hsmBroker.backend: external` plus
  `hsmBroker.pkcs11.libraryPath` and `pinSecret.name` wire this
  in.
- The HSM PIN lives in AWS Secrets Manager, mounted as the
  `pin` key of `aether-hsm-pin` Secret via the Secrets Store CSI
  Driver. Custodians rotate via the procedure in
  [key-ceremony.md](key-ceremony.md).

### S3 — audit log offsite

- Bucket with Object Lock enabled in **Compliance mode**, default
  retention 3 years. Compliance mode means even the bucket owner
  cannot shorten retention.
- KMS encryption with a customer-managed CMK that the audit role
  (and only the audit role) can use to write.
- Cross-region replication to a second qualifying region.
- Lifecycle policy archives objects to Glacier after 90 days
  (cold-storage tiering — operator decision).

### Networking

- Private subnets for the cluster; no node has a public IP.
- ALB in the public subnets terminates HTTPS and mTLS for ES2+.
- Network ACLs deny all egress except: AWS API endpoints (HTTPS),
  RDS (5432 to private subnet), CloudHSM endpoints, the WORM S3
  bucket (via VPC endpoint), and your CI's OCSP responder.
- VPC Flow Logs on. The auditor will look for these.

## Deployment

The `deployments/helm/aether/` chart drives the install. Sample
production override (file the operator owns, kept out of this
repo):

```yaml
postgres:
  enabled: false
postgresUrl: "postgres://aether:$PG_PASSWORD@aether-prod.cluster-xyz.us-east-2.rds.amazonaws.com:5432/aether?sslmode=require"

hsmBroker:
  backend: external
  pkcs11:
    libraryPath: /opt/cloudhsm/lib/libcloudhsm_pkcs11.so
    slot: "0"
    pinSecret:
      name: aether-hsm-pin   # populated via Secrets Store CSI

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
  className: alb
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTPS":443}]'
    alb.ingress.kubernetes.io/ssl-policy: ELBSecurityPolicy-TLS13-1-2-2021-06
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:us-east-2:...:certificate/...
  host: rsp.your-mvno.com
  tls:
    enabled: true
    secretName: aether-tls

global:
  imageRegistry: 123456789012.dkr.ecr.us-east-2.amazonaws.com/aether

metrics:
  prometheusAnnotations: true
```

## Observability

- Prometheus scrapes the chart's `/metrics` endpoints (the chart
  emits the right annotations).
- Alert rules and Prometheus Operator manifests ship under
  [deployments/observability/](https://github.com/ajamous/aether/tree/main/deployments/observability).
  Two flavours: vanilla Prometheus rules YAML for adopters
  running plain Prometheus, plus `PrometheusRule` and
  `ServiceMonitor` CRDs for kube-prometheus-stack adopters.
  When deploying via Helm, set
  `observability.prometheusOperator.enabled: true` to render
  the CRDs in-cluster.
- Implemented alerts (11): audit chain integrity broken (Sev-1),
  audit metrics scrape failing (Sev-2), cert expiring < 30 days
  (Sev-3) / < 7 days (Sev-2) / expired (Sev-1), service down,
  service crash-looping, HSM broker unhealthy (Sev-1), HSM Sign
  p99 latency > 250ms (Sev-2), ES2+ 401 spike per reason
  (Sev-2), Postgres connections exhausted.
- Grafana dashboard JSON: planned, dependent on having a
  Grafana instance to validate against.
- CloudWatch Logs Insights queries pre-baked for the audit
  evidence pack:
  - "All Sign calls in the last 30 days, by key id"
  - "All HTTP 401 from authenticateClient, by source IP"
  - "All cert rotations, by operator"

## Backup and disaster recovery

- RDS automated backups: 35-day retention.
- Manual snapshot before every Aether minor-version upgrade.
- Audit log offsite: nightly dump → WORM S3 + cross-region
  replication.
- Tested annually: full restore in a separate VPC, run
  `/v1/verify`, confirm `ok=true` against the production tail
  hash recorded in the daily timeline anchor (see
  [audit-retention.md](audit-retention.md)).

## Cost ballpark

For an MVNO at the lower end of the SM-DP+ traffic profile:

- EKS control plane: ~$73/mo
- 3 × m6i.xlarge nodes: ~$430/mo
- RDS db.m6g.large Multi-AZ: ~$200/mo
- CloudHSM (2 × hsm.m5.xlarge): ~$2,200/mo  ← dominant cost
- S3 + Glacier transitions: <$50/mo for normal traffic
- ALB: ~$25/mo + traffic

CloudHSM is the dominant cost. Operators with on-prem HSMs already
in service can skip this and run a Thales Luna or Utimaco unit
instead; the hsm-broker is identical.

## What this reference does NOT include

- Multi-region active-active. Single region is correct for SAS-SM
  baseline; multi-region is a Phase 6 follow-up.
- Cost optimisation past the basic shape.
- The CloudHSM activation ceremony. Terraform brings the
  cluster up; the two-person key-ceremony procedure in
  [key-ceremony.md](key-ceremony.md) takes it from there.
- IRSA trust-policy attachment. The Terraform module creates
  the IAM roles but uses placeholder trust policies; wiring
  the EKS OIDC provider's actual ARN is a documented
  post-deploy step. See the module's README.

The Terraform modules implementing this topology live under
`deployments/terraform/aws/`. See that directory's README for
inputs, outputs, and the `examples/full` canonical wiring.

## Cross-references

- [Reference GCP deployment](reference-gcp.md) — same shape on GCP
- [Reference Azure deployment](reference-azure.md) — same shape on Azure
- [Reference on-prem deployment](reference-onprem.md) — same shape on-prem
- [Helm chart](https://github.com/ajamous/aether/tree/main/deployments/helm/aether)
- [Gap analysis](gap-analysis.md) — what each component satisfies
- [Key ceremony](key-ceremony.md) — runs against the CloudHSM
  cluster
- [RBAC](rbac.md) — IRSA-mapped roles
- [Audit retention](audit-retention.md) — WORM-bucket details
