output "vnet_id" {
  value = azurerm_virtual_network.this.id
}

output "vnet_name" {
  value = azurerm_virtual_network.this.name
}

output "aks_subnet_id" {
  value = azurerm_subnet.aks.id
}

output "data_subnet_id" {
  value = azurerm_subnet.data.id
}

output "postgres_dns_zone_id" {
  value = azurerm_private_dns_zone.postgres.id
}

output "log_analytics_workspace_id" {
  value = azurerm_log_analytics_workspace.this.id
}
