# Aether Azure Terraform module

IaC for the Azure reference deployment described in
[`docs/sas-sm/reference-azure.md`](https://github.com/ajamous/aether/blob/main/docs/sas-sm/reference-azure.md).
Stands up VNet + AKS (private cluster, Workload Identity) +
Postgres Flexible Server (zone-redundant HA) + Managed HSM
(FIPS 140-3 Level 3) + immutable Blob Storage in one
`terraform apply`.

## Status

| Piece                                              | Status      |
| -------------------------------------------------- | ----------- |
| Resource group + tags                              | Implemented |
| VNet + AKS subnet + delegated data subnet          | Implemented |
| NSG with default-deny inbound, VNet Flow Logs (365d) | Implemented |
| Log Analytics workspace + traffic analytics        | Implemented |
| User-assigned managed identities (audit, hsm-broker) | Implemented |
| AKS (private cluster, Workload Identity, OIDC issuer, autoscaler) | Implemented |
| Postgres Flexible Server zone-redundant HA + 35-day backup + AAD auth | Implemented |
| Managed HSM (FIPS 140-3 Level 3) — provisioned     | Implemented |
| Storage account + immutable container (locked time-based retention) + GZRS | Implemented |
| Workload Identity federated credentials            | Manual post-deploy step (see "Post-deploy Workload Identity wiring") |
| Managed HSM security-domain ceremony               | Manual two-person procedure per `docs/sas-sm/key-ceremony.md` |
| Application Gateway / Front Door + WAF             | Out of scope — operator owns ingress; chart's `ingress.enabled=true` does the rest |
| `terraform validate` clean                         | CI gate via `.github/workflows/ci.yml` `terraform-validate` job |
| `terraform plan` against a real subscription       | Operator-driven |

## Prerequisites

- Terraform 1.5+
- `az` CLI authenticated against the target subscription, OR
  `ARM_*` env vars set for service-principal auth
- A subscription with Managed HSM allowlisted in your region
  (Managed HSM is only available in a subset of regions; verify
  before you build)
- IAM permissions to create networks, AKS, Postgres flexible
  servers, Key Vaults, Managed HSMs, storage accounts, and
  user-assigned managed identities
- The AAD object IDs of at least one Managed HSM administrator
  (the people who will run the security-domain ceremony)

## Quick start

```bash
cd deployments/terraform/azure/examples/full
az login
terraform init
terraform plan \
    -var=name_prefix=aether-prod \
    -var='managed_hsm_admin_object_ids=["<your-aad-object-id>"]'
terraform apply
```

Once `terraform apply` completes:

```bash
az aks get-credentials --name aether-prod-aks \
    --resource-group aether-prod-rg
```

Then run the Managed HSM security-domain ceremony per
[key-ceremony.md](https://github.com/ajamous/aether/blob/main/docs/sas-sm/key-ceremony.md),
bind the chart's ServiceAccounts via federated credentials (see
below), load identity certs into a Secret, and `helm install`
Aether using the values in the example's `next_steps` output
(`examples/full/main.tf`).

## Module layout

```
.
├── README.md               — this file
├── versions.tf             — Terraform + provider version pins
├── providers.tf            — default azurerm/azuread provider config
├── variables.tf            — top-level inputs
├── main.tf                 — wires the submodules together
├── outputs.tf              — endpoints and IDs the operator needs
├── modules/
│   ├── network/            — VNet, subnets, NSG, Log Analytics, Flow Logs
│   ├── iam/                — audit + hsm-broker user-assigned managed identities
│   ├── aks/                — AKS private cluster, Workload Identity, OIDC issuer
│   ├── postgres/           — Postgres Flexible Server zone-redundant HA + Key Vault for password
│   ├── hsm/                — Managed HSM (FIPS 140-3 L3) — provision only
│   └── storage/            — Storage account + immutable container (locked) + GZRS
└── examples/
    └── full/               — canonical end-to-end deploy
```

## Production values pinned by this module

These are policy, not preference. The module does not let you
change them via variables because doing so would weaken the
SAS-SM posture.

- VNet Flow Logs: enabled, 365-day retention, traffic analytics
  enabled with 10-minute interval
- Log Analytics workspace: 365-day retention
- AKS: private cluster (no public API endpoint), Workload
  Identity + OIDC issuer enabled, Container Insights via OMS
  agent on, Azure Policy enabled, network policy = azure
- Postgres Flexible Server: zone-redundant HA (mode =
  ZoneRedundant), geo-redundant backups, 35-day retention, AAD
  authentication enabled, public network access disabled,
  log_connections + log_disconnections on
- Managed HSM: FIPS 140-3 Level 3, 90-day soft-delete retention,
  purge protection enabled, public network access disabled
- Storage account: GZRS replication, TLS 1.2 minimum, HTTPS-only,
  shared access keys disabled, default OAuth authentication,
  versioning + change feed + 365-day soft-delete on, container
  uses a **Locked** time-based immutability policy
- All resources tagged `aether.managed-by=terraform`

## What this module does NOT do

- **It does not run the Managed HSM security-domain ceremony.**
  Terraform creates the resource; activation requires the
  two-person ceremony documented in
  [docs/sas-sm/key-ceremony.md](https://github.com/ajamous/aether/blob/main/docs/sas-sm/key-ceremony.md).
- **It does not deploy Aether itself.** The Helm chart is a
  separate, post-deploy step. See the chart's README.
- **It does not configure your IdP or OIDC client for the admin
  UI.** That lives wherever your AAD or external IdP is.
- **It does not configure ingress.** The chart's
  `ingress.enabled=true` plus your operator-supplied Application
  Gateway / Front Door do that.
- **It does not handle multi-region active-active.** Single
  region is correct for SAS-SM baseline; multi-region active-
  active is a Phase 6 platform follow-up. (The audit storage
  account is GZRS, which is in scope here.)
- **It does not set up CMEK on the storage account.** Today the
  audit storage uses Microsoft-managed keys; switching to
  Customer-Managed Keys backed by the Managed HSM is a follow-up
  that is only sensible AFTER the security-domain ceremony has
  unlocked the data plane.
- **It does not back up your Terraform state.** Configure a
  remote backend (Azure Storage with state-locking via blob
  lease) before deploying anything you care about.

## Post-deploy Workload Identity wiring

This module creates two user-assigned managed identities (audit
and hsm-broker) but does not bind them to Kubernetes
ServiceAccounts. The Helm chart names its ServiceAccounts
`<release>-aether`, and that release name is not known until
`helm install` runs. After `terraform apply` and `helm install`:

```bash
RG=aether-prod-rg
NS=aether
RELEASE=aether
ISSUER=$(terraform -chdir=examples/full output -raw aks_oidc_issuer_url 2>/dev/null \
        || az aks show --name aether-prod-aks --resource-group "$RG" --query oidcIssuerProfile.issuerUrl -o tsv)

az identity federated-credential create \
    --name aether-audit-fc \
    --identity-name aether-prod-audit-id \
    --resource-group "$RG" \
    --issuer "$ISSUER" \
    --subject "system:serviceaccount:${NS}:${RELEASE}-aether"

az identity federated-credential create \
    --name aether-hsm-broker-fc \
    --identity-name aether-prod-hsm-broker-id \
    --resource-group "$RG" \
    --issuer "$ISSUER" \
    --subject "system:serviceaccount:${NS}:${RELEASE}-aether"
```

Then annotate the chart's ServiceAccounts with
`azure.workload.identity/client-id`. A future iteration of this
module will read the OIDC issuer from the AKS data source and
wire the federated credentials automatically; today it's a manual
step so the module stays a single `terraform apply` rather than
requiring a two-pass deploy.

## Post-deploy security-domain ceremony

The Managed HSM is provisioned in a "Provisioned" but inactive
state. The hsm-broker cannot reach the data plane until a quorum
of administrators downloads, decrypts, and restores the security
domain. The ceremony itself is documented in
[`docs/sas-sm/key-ceremony.md`](https://github.com/ajamous/aether/blob/main/docs/sas-sm/key-ceremony.md);
the relevant Azure-specific calls are roughly:

```bash
# Download security domain (quorum required to decrypt later)
az keyvault security-domain download \
    --hsm-name aether-prod-mhsm \
    --sd-wrapping-keys cert1.pem cert2.pem cert3.pem \
    --sd-quorum 2 \
    --security-domain-file aether-prod-sd.json

# Generate the Aether identity keys (post-ceremony)
az keyvault key create \
    --hsm-name aether-prod-mhsm \
    --name smdp-plus-id \
    --kty EC --curve P-256 --ops sign verify \
    --protection hsm
```

Then assign the local-RBAC role for the hsm-broker:

```bash
az keyvault role assignment create \
    --hsm-name aether-prod-mhsm \
    --role "Managed HSM Crypto User" \
    --assignee <hsm-broker-principal-id> \
    --scope /keys
```

## Validation

CI runs `terraform fmt -check` and `terraform validate` on every
PR (see `.github/workflows/ci.yml`'s `terraform-validate` job;
the Azure cell is part of the matrix). Adopters running locally:

```
terraform fmt -recursive -check
terraform init
terraform validate
```

A successful `terraform plan` against a real Azure subscription
is the operator's responsibility; the module's validation only
proves syntactic and type correctness.
