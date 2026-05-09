# Azure Key Vault Managed HSM — FIPS 140-3 Level 3 single-tenant
# HSM. The SAS-SM-appropriate Azure offering.
#
# After terraform apply the HSM is provisioned but NOT activated.
# Activation requires the security-domain ceremony — a quorum of
# administrators downloads and decrypts the security domain. This
# is the manual two-person procedure documented in
# docs/sas-sm/key-ceremony.md. Terraform stops at the resource
# create; humans take it from there.

resource "azurerm_key_vault_managed_hardware_security_module" "this" {
  name                = "${var.name_prefix}-mhsm"
  resource_group_name = var.resource_group_name
  location            = var.location
  sku_name            = "Standard_B1"
  tenant_id           = var.tenant_id

  # Administrators after the security-domain ceremony. Cannot be
  # empty at create time; supply at least one bootstrap admin in
  # var.managed_hsm_admin_object_ids.
  admin_object_ids = var.admin_object_ids

  purge_protection_enabled   = true
  soft_delete_retention_days = 90

  public_network_access_enabled = false

  tags = var.tags
}

# RBAC role assignment for the hsm-broker managed identity.
# Managed HSM uses local RBAC, separate from Azure RBAC. The
# concrete role assignment (e.g. "Managed HSM Crypto User" on the
# /keys scope) is created at the data plane and is therefore
# part of the post-deploy key-ceremony procedure — the data
# plane is unavailable until security-domain restore completes.
#
# This terraform creates a placeholder Azure RBAC role assignment
# at the control plane that lets the hsm-broker identity invoke
# `Microsoft.KeyVault/managedHsm/keys/read` once the ceremony is
# done. Full local-RBAC role assignments are documented in the
# README's "Post-deploy security-domain ceremony" section.
resource "azurerm_role_assignment" "hsm_broker_reader" {
  scope                = azurerm_key_vault_managed_hardware_security_module.this.id
  role_definition_name = "Reader"
  principal_id         = var.hsm_broker_principal_id
}
