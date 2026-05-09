# Audit-log offsite storage — Azure Storage account with
# immutable container (time-based retention policy, locked) +
# Geo-Zone-Redundant Storage + Microsoft-managed key encryption
# (CMEK-from-Managed-HSM is a reasonable upgrade once the
# Managed HSM is activated; today this module ships with
# service-managed encryption to keep the create-path automatable).
#
# Like AWS S3 Object Lock Compliance and GCS Bucket Lock
# Compliance, a locked time-based retention policy on Azure Blob
# Storage cannot be shortened — even by the storage account
# owner.

resource "random_string" "suffix" {
  length  = 6
  upper   = false
  special = false
}

resource "azurerm_storage_account" "audit" {
  name                = "${replace(var.name_prefix, "-", "")}audit${random_string.suffix.result}"
  resource_group_name = var.resource_group_name
  location            = var.location

  account_tier             = "Standard"
  account_replication_type = "GZRS" # Geo-Zone-Redundant
  account_kind             = "StorageV2"

  min_tls_version                 = "TLS1_2"
  https_traffic_only_enabled      = true
  shared_access_key_enabled       = false
  public_network_access_enabled   = false
  allow_nested_items_to_be_public = false
  default_to_oauth_authentication = true

  blob_properties {
    versioning_enabled  = true
    change_feed_enabled = true

    delete_retention_policy {
      days = 365
    }

    container_delete_retention_policy {
      days = 365
    }
  }

  tags = var.tags
}

resource "azurerm_storage_container" "audit" {
  name                  = "audit"
  storage_account_id    = azurerm_storage_account.audit.id
  container_access_type = "private"
}

# Time-based retention policy in Locked state — Compliance-grade
# immutability. Once locked, retention can be EXTENDED but never
# shortened, and the policy itself cannot be removed.
resource "azurerm_storage_container_immutability_policy" "audit" {
  storage_container_resource_manager_id = azurerm_storage_container.audit.id

  immutability_period_in_days         = var.retention_years * 365
  protected_append_writes_all_enabled = false
  locked                              = true
}

# Audit pod gets Storage Blob Data Contributor on the container
# only — write objects, read for verify, no delete (the locked
# retention policy would block delete anyway, but role separation
# is good hygiene).
resource "azurerm_role_assignment" "audit_writer" {
  scope                = azurerm_storage_container.audit.id
  role_definition_name = "Storage Blob Data Contributor"
  principal_id         = var.audit_principal_id
}
