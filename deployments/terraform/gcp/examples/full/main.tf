# Canonical example: deploy the Aether GCP reference into a
# single project, single region, single Aether release.
#
# To use:
#   1. cd deployments/terraform/gcp/examples/full
#   2. terraform init
#   3. terraform plan -var=project_id=your-gcp-project -var=region=us-central1
#   4. terraform apply
#   5. After the cluster is up, configure kubectl:
#        gcloud container clusters get-credentials aether-prod-gke \
#            --region us-central1 --project your-gcp-project
#   6. Run a key ceremony to populate the production identity keys
#      against the Cloud HSM key ring (manual; see
#      docs/sas-sm/key-ceremony.md).
#   7. Bind each Helm-chart-created Kubernetes ServiceAccount to
#      the matching GCP SA via Workload Identity (see the parent
#      README's "Post-deploy Workload Identity wiring" section).
#   8. helm install aether ../../../../helm/aether -f values-prod.yaml
#
# values-prod.yaml is operator-supplied; it points at the Cloud
# SQL connection name, Cloud HSM key ring, and audit bucket from
# this module's outputs.

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0.0"
    }
  }
}

variable "project_id" {
  type = string
}

variable "region" {
  type    = string
  default = "us-central1"
}

variable "name_prefix" {
  type    = string
  default = "aether-prod"
}

provider "google" {
  project = var.project_id
  region  = var.region
}

module "aether" {
  source = "../.."

  project_id  = var.project_id
  region      = var.region
  name_prefix = var.name_prefix
  environment = "prod"
}

output "next_steps" {
  value = <<-EOT
    Aether GCP reference deployment is up.

    1. Configure kubectl:
         gcloud container clusters get-credentials ${module.aether.gke_cluster_name} \
             --region ${var.region} --project ${var.project_id}
    2. Run the key ceremony (manual, two-person) against the
       Cloud HSM key ring ${module.aether.hsm_keyring_id} per
       docs/sas-sm/key-ceremony.md.
    3. Bind each Aether ServiceAccount to its matching GCP SA via
       Workload Identity:
         gcloud iam service-accounts add-iam-policy-binding ${module.aether.audit_sa_email} \
             --role roles/iam.workloadIdentityUser \
             --member "serviceAccount:${var.project_id}.svc.id.goog[aether/<release>-aether]"
         gcloud iam service-accounts add-iam-policy-binding ${module.aether.hsm_broker_sa_email} \
             --role roles/iam.workloadIdentityUser \
             --member "serviceAccount:${var.project_id}.svc.id.goog[aether/<release>-aether]"
    4. Helm install Aether with values pointing at:
         postgres connection name: ${module.aether.cloudsql_connection_name}
         hsm broker key ring:      ${module.aether.hsm_keyring_id}
         audit bucket:             ${module.aether.audit_bucket_name}
  EOT
}
