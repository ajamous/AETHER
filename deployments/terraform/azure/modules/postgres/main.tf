# Azure Database for PostgreSQL Flexible Server, zone-redundant
# HA, geo-redundant backups, 35-day retention, private only.

data "azurerm_client_config" "current" {}

resource "random_password" "aether" {
  length      = 32
  special     = true
  min_special = 4
  # Postgres rejects some specials in passwords passed via the
  # API; restrict to a safe set.
  override_special = "!#$%&*-_=+[]{}<>?"
}

# Key Vault to hold the master password. Separate from the
# Managed HSM (which only stores cryptographic keys, not blobs).
resource "azurerm_key_vault" "this" {
  name                = "${var.name_prefix}-pgkv"
  resource_group_name = var.resource_group_name
  location            = var.location
  tenant_id           = data.azurerm_client_config.current.tenant_id
  sku_name            = "standard"

  rbac_authorization_enabled    = true
  purge_protection_enabled      = true
  soft_delete_retention_days    = 90
  public_network_access_enabled = false

  network_acls {
    default_action = "Deny"
    bypass         = "AzureServices"
  }

  tags = var.tags
}

# Operator running terraform apply needs Key Vault Secrets Officer
# to write the secret below. Adopters can adjust if they run from
# a service principal with different role grants.
resource "azurerm_role_assignment" "kv_secrets_officer" {
  scope                = azurerm_key_vault.this.id
  role_definition_name = "Key Vault Secrets Officer"
  principal_id         = data.azurerm_client_config.current.object_id
}

resource "azurerm_key_vault_secret" "aether_password" {
  name         = "aether-postgres-password"
  value        = random_password.aether.result
  key_vault_id = azurerm_key_vault.this.id

  depends_on = [azurerm_role_assignment.kv_secrets_officer]
}

resource "azurerm_postgresql_flexible_server" "this" {
  name                = "${var.name_prefix}-pg"
  resource_group_name = var.resource_group_name
  location            = var.location
  version             = var.postgres_version

  administrator_login    = "aether"
  administrator_password = random_password.aether.result

  sku_name   = var.sku_name
  storage_mb = var.storage_mb

  delegated_subnet_id = var.delegated_subnet_id
  private_dns_zone_id = var.private_dns_zone_id

  public_network_access_enabled = false

  backup_retention_days        = var.backup_retention_days
  geo_redundant_backup_enabled = true

  high_availability {
    mode                      = "ZoneRedundant"
    standby_availability_zone = "2"
  }

  authentication {
    active_directory_auth_enabled = true
    password_auth_enabled         = true
    tenant_id                     = data.azurerm_client_config.current.tenant_id
  }

  tags = var.tags

  lifecycle {
    ignore_changes = [zone, high_availability[0].standby_availability_zone]
  }
}

resource "azurerm_postgresql_flexible_server_database" "aether" {
  name      = "aether"
  server_id = azurerm_postgresql_flexible_server.this.id
  collation = "en_US.utf8"
  charset   = "UTF8"
}

# Audit logging — auditor expects to see PG audit configured.
resource "azurerm_postgresql_flexible_server_configuration" "log_connections" {
  name      = "log_connections"
  server_id = azurerm_postgresql_flexible_server.this.id
  value     = "on"
}

resource "azurerm_postgresql_flexible_server_configuration" "log_disconnections" {
  name      = "log_disconnections"
  server_id = azurerm_postgresql_flexible_server.this.id
  value     = "on"
}
