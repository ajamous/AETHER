# AKS — private cluster with Workload Identity + OIDC issuer.
#
# Production posture:
#   - Private cluster (no public API endpoint)
#   - Workload Identity + OIDC issuer enabled (the only way pods
#     get Azure AD identity)
#   - Azure Monitor + Container Insights addon (audit-relevant)
#   - Autoscaler enabled
#   - Authorized IP ranges restrict which CIDRs can reach the API
#
# The `azure_active_directory_role_based_access_control` block is
# left to the operator to configure — they own the AAD tenancy.

resource "azurerm_kubernetes_cluster" "this" {
  name                = "${var.name_prefix}-aks"
  location            = var.location
  resource_group_name = var.resource_group_name
  dns_prefix          = "${var.name_prefix}-aks"
  kubernetes_version  = var.kubernetes_version

  # Private cluster: API server has no public IP.
  private_cluster_enabled             = true
  private_cluster_public_fqdn_enabled = false

  # Workload Identity + OIDC issuer.
  oidc_issuer_enabled       = true
  workload_identity_enabled = true

  default_node_pool {
    name                         = "system"
    vm_size                      = var.node_vm_size
    vnet_subnet_id               = var.aks_subnet_id
    auto_scaling_enabled         = true
    min_count                    = var.node_count_min
    max_count                    = var.node_count_max
    os_disk_type                 = "Ephemeral"
    only_critical_addons_enabled = false
    type                         = "VirtualMachineScaleSets"
    upgrade_settings {
      max_surge = "33%"
    }
  }

  identity {
    type = "SystemAssigned"
  }

  network_profile {
    network_plugin    = "azure"
    network_policy    = "azure"
    load_balancer_sku = "standard"
  }

  api_server_access_profile {
    authorized_ip_ranges = var.authorized_ip_ranges
  }

  oms_agent {
    log_analytics_workspace_id      = var.log_analytics_workspace_id
    msi_auth_for_monitoring_enabled = true
  }

  azure_policy_enabled = true

  tags = var.tags
}
