# Canonical example: deploy the Aether Azure reference into a
# single subscription, single region, single Aether release.
#
# To use:
#   1. cd deployments/terraform/azure/examples/full
#   2. az login                  (or set ARM_* env vars for SP auth)
#   3. terraform init
#   4. terraform plan -var=name_prefix=aether-prod \
#                     -var='managed_hsm_admin_object_ids=["<your-aad-object-id>"]'
#   5. terraform apply
#   6. After AKS is up, configure kubectl:
#        az aks get-credentials --name aether-prod-aks \
#            --resource-group aether-prod-rg
#   7. Run the security-domain ceremony against the Managed HSM
#      (manual two-person procedure; see docs/sas-sm/key-ceremony.md).
#   8. Bind each Helm-chart-created Kubernetes ServiceAccount to
#      the matching managed identity via Workload Identity
#      federated credentials (see the parent README's
#      "Post-deploy Workload Identity wiring" section).
#   9. helm install aether ../../../../helm/aether -f values-prod.yaml
#
# values-prod.yaml is operator-supplied; it points at the
# Postgres FQDN, Managed HSM URI, and audit storage account from
# this module's outputs.

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 4.0.0"
    }
  }
}

provider "azurerm" {
  features {
    key_vault {
      purge_soft_delete_on_destroy    = false
      recover_soft_deleted_key_vaults = true
    }
  }
}

variable "name_prefix" {
  type    = string
  default = "aether-prod"
}

variable "location" {
  type    = string
  default = "westeurope"
}

variable "managed_hsm_admin_object_ids" {
  description = "AAD object IDs of the principals who will run the security-domain ceremony."
  type        = list(string)
}

module "aether" {
  source = "../.."

  name_prefix                  = var.name_prefix
  location                     = var.location
  environment                  = "prod"
  managed_hsm_admin_object_ids = var.managed_hsm_admin_object_ids
}

output "next_steps" {
  value = <<-EOT
    Aether Azure reference deployment is up.

    1. Configure kubectl:
         az aks get-credentials --name ${module.aether.aks_cluster_name} \
             --resource-group ${module.aether.resource_group_name}
    2. Run the Managed HSM security-domain ceremony per
       docs/sas-sm/key-ceremony.md against ${module.aether.managed_hsm_uri}.
    3. Bind each Aether ServiceAccount to its matching managed
       identity via federated credentials. Example for the audit pod:
         az identity federated-credential create \\
             --name aether-audit-fc \\
             --identity-name ${var.name_prefix}-audit-id \\
             --resource-group ${module.aether.resource_group_name} \\
             --issuer ${module.aether.aks_oidc_issuer_url} \\
             --subject "system:serviceaccount:aether:<release>-aether"
    4. Helm install Aether with values pointing at:
         postgres FQDN:        ${module.aether.postgres_fqdn}
         managed HSM URI:      ${module.aether.managed_hsm_uri}
         audit storage:        ${module.aether.audit_storage_account_name}/${module.aether.audit_container_name}
  EOT
}
