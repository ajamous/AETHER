data "azurerm_client_config" "current" {}

locals {
  default_tags = merge(
    {
      "aether.environment" = var.environment
      "aether.managed-by"  = "terraform"
      "aether.module"      = "deployments/terraform/azure"
    },
    var.tags,
  )
}

resource "azurerm_resource_group" "this" {
  name     = "${var.name_prefix}-rg"
  location = var.location
  tags     = local.default_tags
}

module "network" {
  source = "./modules/network"

  resource_group_name = azurerm_resource_group.this.name
  location            = var.location
  name_prefix         = var.name_prefix
  vnet_cidr           = var.vnet_cidr
  aks_subnet_cidr     = var.aks_subnet_cidr
  data_subnet_cidr    = var.data_subnet_cidr
  tags                = local.default_tags
}

module "iam" {
  source = "./modules/iam"

  resource_group_name = azurerm_resource_group.this.name
  location            = var.location
  name_prefix         = var.name_prefix
  tags                = local.default_tags
}

module "aks" {
  source = "./modules/aks"

  resource_group_name        = azurerm_resource_group.this.name
  location                   = var.location
  name_prefix                = var.name_prefix
  kubernetes_version         = var.aks_kubernetes_version
  node_vm_size               = var.aks_node_vm_size
  node_count_min             = var.aks_node_count_min
  node_count_max             = var.aks_node_count_max
  aks_subnet_id              = module.network.aks_subnet_id
  authorized_ip_ranges       = var.aks_authorized_ip_ranges
  log_analytics_workspace_id = module.network.log_analytics_workspace_id
  tags                       = local.default_tags
}

module "postgres" {
  source = "./modules/postgres"

  resource_group_name   = azurerm_resource_group.this.name
  location              = var.location
  name_prefix           = var.name_prefix
  sku_name              = var.postgres_sku_name
  storage_mb            = var.postgres_storage_mb
  postgres_version      = var.postgres_version
  backup_retention_days = var.postgres_backup_retention_days
  delegated_subnet_id   = module.network.data_subnet_id
  private_dns_zone_id   = module.network.postgres_dns_zone_id
  tags                  = local.default_tags
}

module "hsm" {
  source = "./modules/hsm"

  resource_group_name     = azurerm_resource_group.this.name
  location                = var.location
  name_prefix             = var.name_prefix
  tenant_id               = data.azurerm_client_config.current.tenant_id
  admin_object_ids        = var.managed_hsm_admin_object_ids
  hsm_broker_principal_id = module.iam.hsm_broker_principal_id
  tags                    = local.default_tags
}

module "storage" {
  source = "./modules/storage"

  resource_group_name = azurerm_resource_group.this.name
  location            = var.location
  name_prefix         = var.name_prefix
  retention_years     = var.audit_retention_years
  audit_principal_id  = module.iam.audit_principal_id
  tags                = local.default_tags
}
