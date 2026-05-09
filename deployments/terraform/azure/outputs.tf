output "resource_group_name" {
  description = "Resource group containing all Aether resources."
  value       = azurerm_resource_group.this.name
}

output "vnet_id" {
  value = module.network.vnet_id
}

output "aks_cluster_name" {
  description = "AKS cluster name. `az aks get-credentials --name <this> --resource-group <rg>`."
  value       = module.aks.cluster_name
}

output "aks_oidc_issuer_url" {
  description = "AKS OIDC issuer URL. Used to bind Kubernetes ServiceAccounts to user-assigned managed identities via Workload Identity (manual post-deploy step)."
  value       = module.aks.oidc_issuer_url
}

output "postgres_fqdn" {
  description = "Postgres flexible server FQDN. Goes into the Helm chart's postgresUrl."
  value       = module.postgres.fqdn
}

output "postgres_password_secret_name" {
  description = "Key Vault secret name holding the Postgres aether-user password."
  value       = module.postgres.password_secret_name
}

output "postgres_password_key_vault_name" {
  description = "Key Vault holding the Postgres password secret. (Different from the Managed HSM.)"
  value       = module.postgres.password_key_vault_name
}

output "managed_hsm_uri" {
  description = "Managed HSM data-plane URI. The hsm-broker connects here once the security-domain ceremony completes."
  value       = module.hsm.managed_hsm_uri
}

output "audit_storage_account_name" {
  value = module.storage.storage_account_name
}

output "audit_container_name" {
  value = module.storage.container_name
}

output "audit_principal_id" {
  description = "Object ID of the user-assigned managed identity the audit pod runs as via Workload Identity."
  value       = module.iam.audit_principal_id
}

output "audit_client_id" {
  description = "Client ID of the audit user-assigned managed identity. Used by the chart's ServiceAccount annotation `azure.workload.identity/client-id`."
  value       = module.iam.audit_client_id
}

output "hsm_broker_principal_id" {
  description = "Object ID of the user-assigned managed identity the hsm-broker pod runs as via Workload Identity."
  value       = module.iam.hsm_broker_principal_id
}

output "hsm_broker_client_id" {
  description = "Client ID of the hsm-broker user-assigned managed identity."
  value       = module.iam.hsm_broker_client_id
}
