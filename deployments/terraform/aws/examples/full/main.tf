# Canonical example: deploy the Aether AWS reference into a
# single account, single region, single Aether release.
#
# To use:
#   1. cd deployments/terraform/aws/examples/full
#   2. terraform init
#   3. terraform plan -var=region=us-east-2 -var=name_prefix=aether-prod
#   4. terraform apply
#   5. After the cluster is up, configure kubectl:
#        aws eks update-kubeconfig --name aether-prod --region us-east-2
#   6. Activate CloudHSM (manual, see docs/sas-sm/key-ceremony.md).
#   7. Run a key ceremony to populate the production identity certs.
#   8. helm install aether ../../../../helm/aether -f values-prod.yaml
#
# values-prod.yaml is operator-supplied; it points at the RDS
# endpoint, CloudHSM cluster, and audit bucket from this module's
# outputs.

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0.0"
    }
  }
}

variable "region" {
  type    = string
  default = "us-east-2"
}

variable "name_prefix" {
  type    = string
  default = "aether-prod"
}

module "aether" {
  source = "../.."

  region      = var.region
  name_prefix = var.name_prefix
  environment = "prod"
}

output "next_steps" {
  value = <<-EOT
    Aether AWS reference deployment is up.

    1. Configure kubectl:
         aws eks update-kubeconfig --name ${module.aether.eks_cluster_name} --region ${var.region}
    2. Activate CloudHSM cluster ${module.aether.cloudhsm_cluster_id} via the
       key-ceremony procedure (docs/sas-sm/key-ceremony.md).
    3. Helm install Aether with these values:
         postgresUrl: postgres://aether:<from secret ${module.aether.rds_secret_arn}>@${module.aether.rds_endpoint}/aether?sslmode=require
         hsmBroker.backend: external
         certmgr.mode: production
  EOT
}
