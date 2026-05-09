# Reference deployment — on-prem

A reference topology for an Aether SM-DP+ deployment on-prem, for
operators who already run their own HSM hardware (Thales Luna
SA, Utimaco SecurityServer, etc.) or who have data-residency
constraints that exclude the public clouds. Mirrors the structure
of [reference-aws.md](reference-aws.md) so adopters can compare
shapes side by side.

GSMA SAS-SM accreditation does not constrain you to a specific
geography for on-prem deployments — the constraint is operator
control and physical security of the facility. Your auditor will
inspect the physical site for the HSM ceremony room and the
data-centre for the Aether services.

## Topology

```
                          ┌────────────────────────┐
                          │    Public ingress      │
                          │  (your LB + TLS termi-  │
                          │   nation: F5 / nginx /   │
                          │   HAProxy)              │
                          └───────────┬────────────┘
                                      │ HTTPS + mTLS
                                      ▼
                ┌─────────────────────────────────────┐
                │   Kubernetes cluster                │
                │   (Rancher, OpenShift, k3s, ...)    │
                │   aether namespace                  │
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
│ Postgres 16  │            │ Thales Luna SA   │         │ Object storage │
│ (HA: Patroni │            │  or Utimaco      │         │ with WORM      │
│  / pg_auto_  │            │  SecurityServer  │         │ retention      │
│  failover)   │            │ (FIPS 140-2/3)   │         │ (MinIO with    │
│              │            │ Rack-mounted, on │         │  Object Lock,  │
│              │            │  same network    │         │  or Cloudian/  │
│              │            │  as cluster      │         │  Scality, etc.)│
└──────────────┘            └──────────────────┘         └────────────────┘
```

## Components

### Kubernetes

- Distribution: **Rancher**, **OpenShift**, or **k3s** for
  smaller deployments. Vanilla kubeadm works too. The chart is
  vendor-agnostic.
- Cluster size: 3-node control plane minimum; data-plane node
  count scales with traffic. For an MVNO baseline, 3-5 worker
  nodes (32 GB RAM, 8 vCPU each) is plenty.
- Pod Security Standards: `restricted`. The Helm chart's pod
  security context already meets it.
- Cluster logging to your existing log aggregator (rsyslog,
  Splunk, ELK) — the chart emits structured JSON.
- Service accounts use Kubernetes native RBAC; the chart
  creates a per-release SA bound to least-privileged Roles
  (see [rbac.md](rbac.md)).

### Postgres

- Postgres 16, deployed in HA via **Patroni** + **etcd** or
  **pg_auto_failover** (or your preferred HA stack).
- Storage: enterprise SSDs with on-disk encryption (LUKS or
  vendor-provided self-encrypting drives).
- Backups: WAL-G / pgBackRest streaming to the WORM object
  storage bucket below.
- Connect from the cluster via a `Service` of type
  `ExternalName` or a private network route to the Postgres
  node IPs.
- The Helm chart's `postgres.enabled: false` plus the
  Postgres connection string in `postgresUrl` wires this in.

### HSM — Thales Luna SA / Utimaco SecurityServer

- **Thales Luna SA**: rack-mounted appliance, 1U or 2U.
  PKCS#11 client library is `libCryptoki2.so` from the Luna
  Client SDK.
- **Utimaco SecurityServer**: rack-mounted, the PKCS#11
  library is `libcs_pkcs11_R3.so` from Utimaco's PKCS#11
  Provider package.
- HA: deploy two HSMs of the same model on the same network
  segment as the cluster; the vendor SDK handles
  load-balancing and failover.
- The PKCS#11 client library is mounted as a volume into the
  hsm-broker pod via a HostPath (the operator stages the
  library on every node) or via an InitContainer that pulls
  it from a private registry.
- The HSM PIN lives in your existing secret store (HashiCorp
  Vault, CyberArk, Conjur) and is mounted via the
  appropriate CSI driver, exposed as the `pin` key of an
  `aether-hsm-pin` Secret. Custodians rotate via the
  procedure in [key-ceremony.md](key-ceremony.md).

The hsm-broker code path is identical to Cloud HSM
deployments — that's the whole point of [ADR 0003](../architecture/adr/0003-pkcs11-abstraction.md).

### Object storage — audit log offsite

- **MinIO** with **Object Lock** in **Compliance mode**, default
  retention 3 years. MinIO's Object Lock implementation is
  S3-compatible and fully WORM under Compliance.
- Alternatives: **Cloudian HyperStore**, **Scality RING**, or
  any S3-compatible storage with a documented Object Lock
  Compliance mode.
- Encryption at rest with the vendor's KMS or your own KMS
  appliance.
- Replication: configure MinIO bucket replication to a second
  data centre or to a tape archive. The exact mechanism
  depends on your ops stack.
- Lifecycle: archive to cheaper tier after 90 days
  (cold-storage tiering — operator decision).

### Networking

- The cluster sits in your data centre's secure VLAN. No public
  IPs on cluster nodes.
- Public ingress via your existing LB (F5, nginx, HAProxy,
  Citrix ADC) terminates HTTPS and mTLS for ES2+. Configure
  the LB to forward client certs as headers if needed (e.g.
  `X-Client-Cert`).
- Firewall rules deny egress except: your own DNS, Postgres,
  HSM endpoints, the WORM object storage, your internal NTP,
  and your CI's OCSP responder (or your offline CRL refresh
  process).
- Network flow logs to your SIEM. The auditor will look for
  them.

## Deployment

The `deployments/helm/aether/` chart drives the install. Sample
production override (file the operator owns, kept out of this
repo):

```yaml
postgres:
  enabled: false
postgresUrl: "postgres://aether:$PG_PASSWORD@aether-pg-vip.dc.local:5432/aether?sslmode=require"

hsmBroker:
  backend: external
  pkcs11:
    libraryPath: /opt/luna/lib/libCryptoki2.so   # or Utimaco
    slot: "0"
    pinSecret:
      name: aether-hsm-pin   # populated via Vault CSI

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
  className: nginx
  host: rsp.your-mvno.local   # or whatever your LB fronts
  tls:
    enabled: true
    secretName: aether-tls

global:
  imageRegistry: registry.dc.local/aether

metrics:
  prometheusAnnotations: true

observability:
  prometheusOperator:
    enabled: true   # if you run kube-prometheus-stack on-prem
```

## Observability

- **Prometheus + Grafana** running on-prem is the typical
  pattern; apply the bundled rules from
  [deployments/observability/](https://github.com/ajamous/aether/tree/main/deployments/observability).
- Alerts route to your existing on-call platform (Pagerduty,
  Opsgenie, your in-house phone tree).
- Centralised logs land in your existing aggregator (Splunk,
  ELK, Graylog). Structured JSON from the Aether services is
  ready for any of these.

## Backup and disaster recovery

- Postgres backups: WAL-G / pgBackRest stream WAL + base
  backups to the WORM object storage. Retention 35 days
  baseline; longer if your jurisdiction requires.
- Manual logical backup before every Aether minor-version
  upgrade.
- Audit log offsite: nightly `pg_dump audit_entries` to the
  WORM bucket plus the daily timeline anchor pattern in
  [audit-retention.md](audit-retention.md).
- Tested annually: full restore in your secondary site, run
  `/v1/verify`, confirm `ok=true` against the production tail
  hash.

For the recovery procedures across single-host failure,
data-centre outage, and database compromise, see
[disaster-recovery.md](disaster-recovery.md).

## Cost ballpark

On-prem CapEx vs cloud OpEx is the major shift. Numbers below
are illustrative for an MVNO baseline; concrete pricing depends
on existing infrastructure, vendor contracts, and amortisation.

CapEx (one-time):
- 5-node Kubernetes cluster (Dell / HPE rack servers): ~$25k
- 2 × Thales Luna SA: ~$40-60k for the pair
- Storage (10 TB enterprise SSD + WORM tier): ~$15k
- Networking + LB licences: ~$20k

OpEx (monthly):
- Power, cooling, rack space: ~$1k
- HSM support contract: ~$300/mo (typically 8% of CapEx
  amortised annually)
- Postgres support (EnterpriseDB or community + ops time):
  varies
- Operations effort: 0.25-0.5 FTE for a small operator

Compared to the cloud references, on-prem trades ~$2,500/mo of
HSM-as-a-service for $40-60k of CapEx that amortises over
3-5 years. For operators already running on-prem
infrastructure, the marginal cost of adding Aether is small.
For greenfield deployments, cloud HSM is usually cheaper for
the first 18-24 months.

## What this reference does NOT include

- Multi-site active-active. Single-site is correct for SAS-SM
  baseline; multi-site is a Phase 6 platform follow-up.
- Specific HA Postgres choice (Patroni vs pg_auto_failover vs
  Crunchy). Pick what your team already runs.
- Specific HSM vendor selection. Thales and Utimaco both pass
  SAS-SM; the platform doesn't care which.
- Pre-built IaC. There is no on-prem-Terraform module under
  `deployments/terraform/` because there isn't a single
  on-prem provider; bring your own.

## Cross-references

- [Reference AWS deployment](reference-aws.md) — same shape on AWS
- [Reference GCP deployment](reference-gcp.md) — same shape on GCP
- [Reference Azure deployment](reference-azure.md) — same shape on Azure
- [Helm chart](https://github.com/ajamous/aether/tree/main/deployments/helm/aether)
- [Gap analysis](gap-analysis.md) — what each component satisfies
- [Key ceremony](key-ceremony.md) — runs against your on-prem HSM
- [Audit retention](audit-retention.md) — WORM-bucket details
