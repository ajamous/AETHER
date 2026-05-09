# Aether AWS Terraform module

IaC for the AWS reference deployment described in
[`docs/sas-sm/reference-aws.md`](https://github.com/ajamous/aether/blob/main/docs/sas-sm/reference-aws.md).
Stands up VPC + EKS + RDS Multi-AZ + CloudHSM + WORM S3 in one
`terraform apply`.

## Status

| Piece                                          | Status      |
| ---------------------------------------------- | ----------- |
| VPC + private/public subnets + NAT + Flow Logs | Implemented |
| IAM roles (EKS cluster, EKS node, audit, hsm-broker) | Implemented |
| EKS cluster + managed node group + Secrets KMS | Implemented |
| RDS Postgres Multi-AZ + KMS CMEK + Secrets Manager | Implemented |
| CloudHSM cluster (HSMs spread across AZs)      | Implemented |
| S3 audit-log bucket with Object Lock Compliance | Implemented |
| Cross-region replica bucket                    | Implemented (skeleton; replication rule wiring needs caller-supplied second-region provider) |
| IRSA OIDC trust policy attachment              | Manual post-deploy step (see "Post-deploy IRSA wiring") |
| ALB + ingress + ACM cert                       | Out of scope — operator owns ingress; chart's `ingress.enabled=true` does the rest |
| `terraform validate` clean                     | CI gate via `.github/workflows/ci.yml` `terraform-validate` job |
| `terraform plan` against a real AWS account    | Operator-driven |

## Prerequisites

- Terraform 1.5+
- AWS CLI configured for the account you want to deploy to
- Two GSMA-qualifying regions chosen (one primary, one for the
  audit-bucket replica — defaults: us-east-2 + eu-west-3)
- Permissions to create VPC, EKS, RDS, CloudHSM, S3, KMS, IAM in
  the target account

## Quick start

```bash
cd deployments/terraform/aws/examples/full
terraform init
terraform plan -var=region=us-east-2 -var=name_prefix=aether-prod
terraform apply
```

Once `terraform apply` completes:

```bash
aws eks update-kubeconfig --name aether-prod --region us-east-2
```

Then activate the CloudHSM cluster following
[docs/sas-sm/key-ceremony.md](https://github.com/ajamous/aether/blob/main/docs/sas-sm/key-ceremony.md),
load your CI-issued identity certs into a Secret, and
`helm install` Aether using the values listed in the example's
`next_steps` output (`examples/full/main.tf`).

## Module layout

```
.
├── README.md             — this file
├── versions.tf           — Terraform + provider version pins
├── providers.tf          — default AWS provider config
├── variables.tf          — top-level inputs
├── main.tf               — wires the submodules together
├── outputs.tf            — endpoints and ARNs the operator needs
├── modules/
│   ├── network/          — VPC, subnets, NAT, Flow Logs
│   ├── iam/              — service IAM roles + IRSA skeletons
│   ├── eks/              — EKS cluster + node group + KMS for Secrets
│   ├── rds/              — Postgres Multi-AZ + KMS CMEK + Secret
│   ├── cloudhsm/         — CloudHSM cluster + HSMs across AZs
│   └── storage/          — S3 WORM bucket + replica + lifecycle
└── examples/
    └── full/             — canonical end-to-end deploy
```

## Production values pinned by this module

These are policy, not preference. The module does not let you
change them via variables because doing so would weaken the
SAS-SM posture.

- VPC Flow Logs: enabled, 365-day retention
- EKS control-plane logs: api, audit, authenticator,
  controllerManager, scheduler — all on
- EKS Secrets envelope encryption: customer-managed KMS key with
  rotation enabled
- RDS: Multi-AZ, encrypted at rest with CMEK, 35-day automated
  backup retention, deletion protection on, Performance Insights
  on, postgresql logs to CloudWatch
- S3 audit bucket: Object Lock **Compliance** mode (immutable
  retention even from bucket owner), KMS-CMEK encryption, public
  access block on, versioning on, lifecycle to Glacier after 90d
- All resources tagged `aether.managed-by=terraform`

## What this module does NOT do

- **It does not run the CloudHSM activation ceremony.** That is a
  manual two-person procedure documented in
  [docs/sas-sm/key-ceremony.md](https://github.com/ajamous/aether/blob/main/docs/sas-sm/key-ceremony.md).
  Terraform brings the cluster up; humans take it from there.
- **It does not deploy Aether itself.** The Helm chart is a
  separate, post-deploy step. See the chart's README.
- **It does not configure your IdP or OIDC.** The admin UI's
  OIDC client lives wherever your IdP is.
- **It does not configure ingress.** The chart's
  `ingress.enabled=true` plus your operator-supplied ALB
  controller and ACM cert do that.
- **It does not handle multi-region active-active.** Single
  region is correct for SAS-SM baseline; multi-region active-
  active is a Phase 6 platform follow-up.
- **It does not back up your Terraform state.** Configure a
  remote backend (S3 + DynamoDB) before deploying anything you
  care about.

## Post-deploy IRSA wiring

This module creates the IAM roles for IRSA but uses placeholder
trust policies (the EKS OIDC provider ARN isn't known until the
cluster is up). After `terraform apply`:

1. Note the EKS cluster's OIDC issuer URL from
   `aws eks describe-cluster --name <name> --query
   cluster.identity.oidc.issuer`.
2. Create the OIDC provider in IAM:
   `aws iam create-open-id-connect-provider --url <issuer> ...`
3. Update the trust policies on the per-service roles to allow
   the matching ServiceAccount under the OIDC provider. The
   per-service ServiceAccounts are created by the Helm chart;
   their names are `<release>-aether`.

A future iteration of this module will read the OIDC issuer
from the EKS data source and wire the trust policies
automatically; today it's a manual step so the module stays a
single `terraform apply` rather than requiring a two-pass
deploy.

## Validation

CI runs `terraform fmt -check` and `terraform validate` on
every PR (see `.github/workflows/ci.yml`'s `terraform-validate`
job). Adopters running locally:

```
terraform fmt -recursive -check
terraform init
terraform validate
```

A successful `terraform plan` against a real account is the
operator's responsibility; the module's validation only proves
syntactic and type correctness.
