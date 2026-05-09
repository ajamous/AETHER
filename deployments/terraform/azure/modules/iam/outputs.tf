output "audit_principal_id" {
  value = azurerm_user_assigned_identity.audit.principal_id
}

output "audit_client_id" {
  value = azurerm_user_assigned_identity.audit.client_id
}

output "audit_identity_id" {
  value = azurerm_user_assigned_identity.audit.id
}

output "hsm_broker_principal_id" {
  value = azurerm_user_assigned_identity.hsm_broker.principal_id
}

output "hsm_broker_client_id" {
  value = azurerm_user_assigned_identity.hsm_broker.client_id
}

output "hsm_broker_identity_id" {
  value = azurerm_user_assigned_identity.hsm_broker.id
}
