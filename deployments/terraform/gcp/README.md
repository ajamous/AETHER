# Aether GCP Terraform module

IaC for the GCP reference deployment described in
[`docs/sas-sm/reference-gcp.md`](https://github.com/ajamous/aether/blob/main/docs/sas-sm/reference-gcp.md).
Stands up VPC + GKE Autopilot + Cloud SQL Postgres (regional HA)
+ Cloud HSM (Cloud KMS HSM-protected keys) + WORM GCS bucket in
one `terraform apply`.

## Status

| Piece                                              | Status      |
| -------------------------------------------------- | ----------- |
| VPC + private subnet + NAT + Flow Logs             | Implemented |
| Service accounts (audit, hsm-broker)               | Implemented |
| GKE Autopilot (private cluster, Workload Identity) | Implemented |
| Cloud SQL Postgres regional HA + CMEK + 35-day backup | Implemented |
| Cloud HSM key ring (HSM protection level)          | Implemented |
| GCS audit bucket with Bucket Lock Compliance       | Implemented |
| Workload Identity binding to chart ServiceAccounts | Manual post-deploy step (see "Post-deploy Workload Identity wiring") |
| External HTTPS LB + ServerTlsPolicy                | Out of scope — operator owns ingress; chart's `ingress.enabled=true` does the rest |
| `terraform validate` clean                         | CI gate via `.github/workflows/ci.yml` `terraform-validate` job |
| `terraform plan` against a real GCP project        | Operator-driven |

## Prerequisites

- Terraform 1.5+
- `gcloud` CLI authenticated against the target project
- A GCP project with billing enabled and the following APIs
  enabled (the module will fail loudly if they are not):
  `compute.googleapis.com`, `container.googleapis.com`,
  `sqladmin.googleapis.com`, `cloudkms.googleapis.com`,
  `secretmanager.googleapis.com`, `servicenetworking.googleapis.com`
- A GSMA-qualifying region chosen (default: `us-central1`)
- IAM permissions to create networks, GKE clusters, Cloud SQL
  instances, KMS key rings, GCS buckets, IAM service accounts

## Quick start

```bash
cd deployments/terraform/gcp/examples/full
terraform init
terraform plan -var=project_id=your-gcp-project -var=region=us-central1
terraform apply
```

Once `terraform apply` completes:

```bash
gcloud container clusters get-credentials aether-prod-gke \
    --region us-central1 --project your-gcp-project
```

Then activate the Cloud HSM key ring via the
[key-ceremony](https://github.com/ajamous/aether/blob/main/docs/sas-sm/key-ceremony.md)
procedure, bind the Helm chart's ServiceAccounts via Workload
Identity (see below), load identity certs into a Secret, and
`helm install` Aether using the values in the example's
`next_steps` output (`examples/full/main.tf`).

## Module layout

```
.
├── README.md               — this file
├── versions.tf             — Terraform + provider version pins
├── providers.tf            — default Google provider config
├── variables.tf            — top-level inputs
├── main.tf                 — wires the submodules together
├── outputs.tf              — endpoints and IDs the operator needs
├── modules/
│   ├── network/            — VPC, subnet, secondary ranges, NAT, Flow Logs
│   ├── iam/                — audit + hsm-broker GCP service accounts
│   ├── gke/                — GKE Autopilot, Workload Identity, private cluster
│   ├── cloudsql/           — Postgres 16 regional HA + CMEK + Secret Manager
│   ├── cloudhsm/           — KMS key ring at HSM protection level
│   └── storage/            — GCS bucket + Bucket Lock Compliance + CMEK
└── examples/
    └── full/               — canonical end-to-end deploy
```

## Production values pinned by this module

These are policy, not preference. The module does not let you
change them via variables because doing so would weaken the
SAS-SM posture.

- VPC subnet Flow Logs: enabled, 5-second aggregation, 50%
  sampling, all metadata
- GKE: private cluster (no public node IPs), Workload Identity
  enabled, Cloud Logging + Cloud Monitoring on, deletion
  protection on, Pod Security Admission `restricted` (the chart
  already meets it)
- Cloud SQL: regional HA, encrypted at rest with CMEK, 35-day
  automated backup retention with PITR, deletion protection on,
  IAM authentication enabled, query insights on, ipv4 disabled
  (private IP via VPC peering)
- Cloud HSM: Cloud KMS key ring with `protection_level = HSM`
  (FIPS 140-2 Level 3); 90-day rotation period
- Audit bucket: Bucket Lock retention policy in **Compliance**
  mode (immutable retention even from the project owner),
  CMEK-encrypted, public access prevention enforced, uniform
  bucket-level access on, versioning on, lifecycle to Coldline
  after 90 days
- All resources labelled `aether-managed-by=terraform`

## What this module does NOT do

- **It does not run the Cloud HSM key ceremony.** That is a
  manual two-person procedure documented in
  [docs/sas-sm/key-ceremony.md](https://github.com/ajamous/aether/blob/main/docs/sas-sm/key-ceremony.md).
  Terraform creates the key ring; humans generate the identity
  keys against it.
- **It does not deploy Aether itself.** The Helm chart is a
  separate, post-deploy step. See the chart's README.
- **It does not configure your IdP or OIDC.** The admin UI's
  OIDC client lives wherever your IdP is.
- **It does not configure ingress.** The chart's
  `ingress.enabled=true` plus your operator-supplied
  External HTTPS LB and managed certificate do that.
- **It does not handle multi-region active-active.** Single
  region is correct for SAS-SM baseline; multi-region active-
  active is a Phase 6 platform follow-up. (The audit bucket can
  be dual-region; that is in scope here.)
- **It does not enable the GCP APIs the resources depend on.**
  Enable them in your project before `terraform apply`. See
  Prerequisites above.
- **It does not back up your Terraform state.** Configure a
  remote backend (GCS bucket + state locking via
  `terraform_remote_state` is not the same — use a GCS backend
  that supports locking) before deploying anything you care
  about.

## Post-deploy Workload Identity wiring

This module creates two GCP service accounts (audit and
hsm-broker) but does not bind them to Kubernetes ServiceAccounts.
The Helm chart names its ServiceAccounts `<release>-aether`, and
that release name is not known until `helm install` runs. After
`terraform apply` and `helm install`:

```bash
PROJECT=your-gcp-project
RELEASE=aether          # or whatever you used with `helm install`
NAMESPACE=aether        # default

gcloud iam service-accounts add-iam-policy-binding \
    aether-audit@${PROJECT}.iam.gserviceaccount.com \
    --role roles/iam.workloadIdentityUser \
    --member "serviceAccount:${PROJECT}.svc.id.goog[${NAMESPACE}/${RELEASE}-aether]"

gcloud iam service-accounts add-iam-policy-binding \
    aether-hsm-broker@${PROJECT}.iam.gserviceaccount.com \
    --role roles/iam.workloadIdentityUser \
    --member "serviceAccount:${PROJECT}.svc.id.goog[${NAMESPACE}/${RELEASE}-aether]"
```

Then annotate the chart's ServiceAccounts with
`iam.gke.io/gcp-service-account`. A future iteration of this
module will read the cluster's Workload Identity pool from a
data source and wire the bindings automatically; today it's a
manual step so the module stays a single `terraform apply` rather
than requiring a two-pass deploy.

## Validation

CI runs `terraform fmt -check` and `terraform validate` on every
PR (see `.github/workflows/ci.yml`'s `terraform-validate` job).
Adopters running locally:

```
terraform fmt -recursive -check
terraform init
terraform validate
```

A successful `terraform plan` against a real GCP project is the
operator's responsibility; the module's validation only proves
syntactic and type correctness.
