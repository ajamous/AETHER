output "storage_account_name" {
  value = azurerm_storage_account.audit.name
}

output "storage_account_id" {
  value = azurerm_storage_account.audit.id
}

output "container_name" {
  value = azurerm_storage_container.audit.name
}

output "blob_endpoint" {
  value = azurerm_storage_account.audit.primary_blob_endpoint
}
