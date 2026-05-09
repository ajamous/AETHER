output "managed_hsm_id" {
  value = azurerm_key_vault_managed_hardware_security_module.this.id
}

output "managed_hsm_name" {
  value = azurerm_key_vault_managed_hardware_security_module.this.name
}

output "managed_hsm_uri" {
  value = azurerm_key_vault_managed_hardware_security_module.this.hsm_uri
}
