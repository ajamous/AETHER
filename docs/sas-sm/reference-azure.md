# Reference deployment — Azure

A reference topology for an Aether SM-DP+ deployment on Microsoft
Azure, sized for an MVNO running through SAS-SM accreditation.
Mirrors the structure of [reference-aws.md](reference-aws.md) and
[reference-gcp.md](reference-gcp.md) so adopters can compare
shapes side by side.

GSMA SAS-SM accreditation specifies the geographic regions where
sensitive processes may run. Azure regions currently qualifying
for GSMA-rated deployments include **West Europe (Netherlands)**,
**North Europe (Ireland)**, **France Central (Paris)**, **Germany
West Central (Frankfurt)**, and others. Confirm the current list
with your auditor before you build; the list does change.

Azure Managed HSM (the FIPS 140-3 Level 3 offering) is only
available in a subset of regions — verify availability before
you pick a region.

## Topology

```
                          ┌────────────────────────┐
                          │    Public ingress      │
                          │  (Application Gateway   │
                          │   + WAF + AKS Ingress)  │
                          └───────────┬────────────┘
                                      │ HTTPS + mTLS
                                      ▼
                ┌─────────────────────────────────────┐
                │   AKS cluster — aether namespace    │
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
┌──────────────────┐         ┌──────────────────┐         ┌────────────────┐
│ Postgres Flex    │         │ Managed HSM      │         │ Storage acct   │
│ Server, zone-    │         │ (single-tenant   │         │ + immutable    │
│ redundant HA,    │         │  HSM, FIPS 140-3 │         │ container,     │
│ private only,    │         │  Level 3)        │         │ Compliance,    │
│ AAD auth         │         │                  │         │ GZRS           │
└──────────────────┘         └──────────────────┘         └────────────────┘
```

## Components

### AKS

- One cluster, one namespace per environment (`aether-prod`,
  `aether-staging`).
- **Private cluster** — the Kubernetes API server has no public
  IP. Access via Bastion, VPN, or authorized-IP-range list.
- Node pool: minimum 3 nodes spread across availability zones in
  the region; `Standard_D4s_v5` (4 vCPU / 16 GiB) is a reasonable
  floor.
- Cluster autoscaler enabled.
- Pod Security Admission policy: `restricted`. The Helm chart's
  pod security context already meets it.
- Container Insights via the OMS agent — Azure Monitor + Log
  Analytics absorb every pod's logs and metrics. Audit-relevant;
  the auditor will look for it.
- **Workload Identity** binds the per-service `ServiceAccount`s
  the chart creates to **user-assigned managed identities**.
  Each service gets *only* the permissions it needs (e.g.
  hsm-broker → "Managed HSM Crypto User" on `/keys`, audit →
  "Storage Blob Data Contributor" on the WORM container).
- Azure Policy add-on enabled for guardrails.

### Postgres

- **Azure Database for PostgreSQL Flexible Server**, version 16,
  zone-redundant high availability (`mode = ZoneRedundant`),
  encryption at rest with Microsoft-managed keys (CMEK from a
  customer-controlled Key Vault is a reasonable upgrade for
  operators with stricter key-rotation requirements).
- SKU: `GP_Standard_D4ds_v5` minimum (4 vCPU, 16 GiB).
- Automated backups: 35-day retention, geo-redundant.
- Point-in-Time Recovery enabled (implicit with Flexible Server).
- AAD authentication enabled — operators authenticate via AAD,
  not local DB users where possible.
- Audit logging: `log_connections=on`, `log_disconnections=on`,
  shipped to Log Analytics via Diagnostic Settings.
- Connectivity: private IP only, via the delegated subnet plus a
  private DNS zone. No public network access.
- The Helm chart's `postgres.enabled: false` plus the FQDN of
  the Flexible Server in `postgresUrl` wires this in.

### Managed HSM

- **Azure Key Vault Managed HSM** — single-tenant, FIPS 140-3
  Level 3. The SAS-SM-appropriate Azure offering (Premium Key
  Vault HSM-backed keys are FIPS 140-2 Level 2 and not
  sufficient for the Standard).
- HA: a Managed HSM instance is internally a 3-node cluster; the
  service handles replication + failover.
- Activation requires the **security-domain ceremony** —
  Terraform creates the resource, but a quorum of administrators
  must download, decrypt, and restore the security domain before
  the data plane is reachable. This is the manual two-person
  procedure documented in [key-ceremony.md](key-ceremony.md).
- Network: private network access only.
- The Aether `hsm-broker` reaches the Managed HSM via its data
  plane URI (`https://<name>.managedhsm.azure.net`) using AAD
  tokens from its user-assigned managed identity. The PKCS#11
  facade for Managed HSM is provided via the `pkcs11-azure`
  shim binary that wraps the data-plane REST API; mount it into
  the pod and point `hsmBroker.pkcs11.libraryPath` at it. The
  chart's `hsmBroker.backend: external` plus `pinSecret.name`
  (which holds an AAD-token-leaning credential, NOT a PIN
  literally) wires this in.

### Storage — audit log offsite

- Storage account, **GZRS** (Geo-Zone-Redundant) replication —
  three zones in the primary region plus async replication to a
  secondary region.
- Container with a **Locked time-based immutability policy**.
  Once locked, retention can be EXTENDED but never shortened, and
  the policy itself cannot be removed. Default retention 3 years
  (configurable). Like AWS S3 Object Lock Compliance and GCS
  Bucket Lock Compliance.
- TLS 1.2 minimum, HTTPS-only, shared access keys disabled,
  default OAuth authentication, public network access disabled.
- Encryption: today the module ships with service-managed keys.
  CMEK backed by the Managed HSM is a reasonable follow-up after
  the security-domain ceremony has unlocked the data plane.
- Versioning + change feed + 365-day soft-delete on, both at the
  container and blob level.

### Networking

- VNet with a delegated subnet for AKS and a separate delegated
  subnet for Postgres Flexible Server (Azure requires this — the
  flexible-server SKU consumes a whole subnet).
- NSG with default-deny inbound from Internet on the AKS subnet.
- VNet Flow Logs via Network Watcher, 365-day retention,
  Traffic Analytics enabled, shipped to a Log Analytics
  workspace.
- Public ingress is the operator's choice — Application Gateway
  v2 with the WAF SKU is the typical answer; the chart's
  `ingress.className: azure-application-gateway` and
  AGIC-installed Ingress controller wire it in.

## Deployment

The `deployments/helm/aether/` chart drives the install. Sample
production override (file the operator owns, kept out of this
repo):

```yaml
postgres:
  enabled: false
postgresUrl: "postgres://aether@<postgres-fqdn>:5432/aether?sslmode=require"
# AAD-auth-issued token rotation handled by an init/sidecar that
# refreshes the connection string; the chart consumes the static
# postgresUrl above when not using AAD.

hsmBroker:
  backend: external
  pkcs11:
    libraryPath: /opt/aether/lib/pkcs11-azure.so
    slot: "0"
    pinSecret:
      name: aether-hsm-credential   # contains AAD token / cert

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
  className: azure-application-gateway
  annotations:
    appgw.ingress.kubernetes.io/ssl-redirect: "true"
  host: rsp.your-mvno.com
  tls:
    enabled: true
    secretName: aether-tls

global:
  imageRegistry: <youracr>.azurecr.io/aether

metrics:
  prometheusAnnotations: true

observability:
  prometheusOperator:
    enabled: true   # if you've installed kube-prometheus-stack
```

## Observability

- Azure Monitor + Log Analytics is the default — Container
  Insights via the OMS agent ships every pod's logs + metrics
  to the workspace this module provisions.
- Alternatively run kube-prometheus-stack in-cluster and apply
  the bundled rules from
  [deployments/observability/](https://github.com/ajamous/aether/tree/main/deployments/observability).
- Azure Monitor alerts for the must-haves
  (see [reference-aws.md](reference-aws.md) §"Observability"
  for the same list adapted to CloudWatch); replace each with
  the equivalent KQL-based Log Analytics alert.
- Pre-baked KQL queries for the audit evidence pack:
  - "All Sign calls in the last 30 days, by key id"
  - "All HTTP 401 from authenticateClient, by source IP"
  - "All cert rotations, by operator"

## Backup and disaster recovery

- Postgres Flexible Server automated backups: 35-day retention,
  geo-redundant.
- Manual on-demand backup before every Aether minor-version
  upgrade.
- Audit log offsite: nightly dump → immutable storage container;
  the GZRS replication provides in-region zone redundancy plus
  reach to a paired region.
- Tested annually: full restore in a separate VNet, run
  `/v1/verify`, confirm `ok=true` against the production tail
  hash recorded in the daily timeline anchor (see
  [audit-retention.md](audit-retention.md)).

See [disaster-recovery.md](disaster-recovery.md) for the recovery
procedure across single-zone, regional, and database-compromise
scenarios.

## Cost ballpark

For an MVNO at the lower end of the SM-DP+ traffic profile
(US-pricing rough numbers; Azure reserved-instance discounts can
shave significantly):

- AKS control plane (Standard tier, with uptime SLA): ~$75/mo
- Worker nodes (3× D4s_v5): ~$300/mo
- Postgres Flexible Server GP_Standard_D4ds_v5 zone-redundant HA: ~$600/mo
- Managed HSM: ~$3,000/mo  ← dominant cost
- Storage account GZRS (audit log volume): <$60/mo for normal traffic
- Application Gateway v2 + WAF: ~$250/mo + traffic
- Log Analytics + Azure Monitor: depends on volume

Managed HSM is the dominant cost, as on AWS and GCP. Operators
with on-prem HSMs already in service can skip this and follow
[reference-onprem.md](reference-onprem.md) instead; the
hsm-broker is identical.

## What this reference does NOT include

- Multi-region active-active. Single region is correct for
  SAS-SM baseline; multi-region is a Phase 6 follow-up.
- Cost optimisation past the basic shape.
- The Managed HSM security-domain ceremony. Terraform creates
  the resource; activation is the manual two-person procedure
  in [key-ceremony.md](key-ceremony.md).
- Workload Identity federated-credential bindings. The Terraform
  module creates the audit and hsm-broker user-assigned managed
  identities but the binding to the chart's Kubernetes
  ServiceAccount (named `<release>-aether`) is documented as a
  post-deploy `az identity federated-credential create` call.
  See the module's README.

The Terraform modules implementing this topology live under
`deployments/terraform/azure/`. See that directory's README for
inputs, outputs, and the `examples/full` canonical wiring.

## Cross-references

- [Reference AWS deployment](reference-aws.md) — same shape on AWS
- [Reference GCP deployment](reference-gcp.md) — same shape on GCP
- [Reference on-prem deployment](reference-onprem.md) — same shape on-prem
- [Helm chart](https://github.com/ajamous/aether/tree/main/deployments/helm/aether)
- [Gap analysis](gap-analysis.md) — what each component satisfies
- [Key ceremony](key-ceremony.md) — runs against the Managed HSM
- [Audit retention](audit-retention.md) — immutable container details
