output "fqdn" {
  value = azurerm_postgresql_flexible_server.this.fqdn
}

output "name" {
  value = azurerm_postgresql_flexible_server.this.name
}

output "password_secret_name" {
  value = azurerm_key_vault_secret.aether_password.name
}

output "password_key_vault_name" {
  value = azurerm_key_vault.this.name
}

output "password_key_vault_id" {
  value = azurerm_key_vault.this.id
}
