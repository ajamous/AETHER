# User-assigned managed identities for Aether services.
#
# Workload Identity binds each Kubernetes ServiceAccount the Helm
# chart creates (named `<release>-aether`) to the matching managed
# identity via a federated-identity-credential. The federated
# credential is post-deploy because the AKS OIDC issuer + chart
# release name aren't known at apply time. See README §"Post-deploy
# Workload Identity wiring" for the `az identity federated-credential
# create` calls.

resource "azurerm_user_assigned_identity" "audit" {
  resource_group_name = var.resource_group_name
  location            = var.location
  name                = "${var.name_prefix}-audit-id"
  tags                = var.tags
}

resource "azurerm_user_assigned_identity" "hsm_broker" {
  resource_group_name = var.resource_group_name
  location            = var.location
  name                = "${var.name_prefix}-hsm-broker-id"
  tags                = var.tags
}
