resource "azurerm_log_analytics_workspace" "this" {
  name                = "${var.name_prefix}-law"
  resource_group_name = var.resource_group_name
  location            = var.location
  sku                 = "PerGB2018"
  retention_in_days   = 365 # SAS-SM gap analysis row "Network and infrastructure security"
  tags                = var.tags
}

resource "azurerm_virtual_network" "this" {
  name                = "${var.name_prefix}-vnet"
  resource_group_name = var.resource_group_name
  location            = var.location
  address_space       = [var.vnet_cidr]
  tags                = var.tags
}

resource "azurerm_subnet" "aks" {
  name                 = "${var.name_prefix}-aks-subnet"
  resource_group_name  = var.resource_group_name
  virtual_network_name = azurerm_virtual_network.this.name
  address_prefixes     = [var.aks_subnet_cidr]

  service_endpoints = [
    "Microsoft.KeyVault",
    "Microsoft.Storage",
  ]
}

resource "azurerm_subnet" "data" {
  name                 = "${var.name_prefix}-data-subnet"
  resource_group_name  = var.resource_group_name
  virtual_network_name = azurerm_virtual_network.this.name
  address_prefixes     = [var.data_subnet_cidr]

  delegation {
    name = "fs"
    service_delegation {
      name = "Microsoft.DBforPostgreSQL/flexibleServers"
      actions = [
        "Microsoft.Network/virtualNetworks/subnets/join/action",
      ]
    }
  }
}

# Private DNS zone for Postgres flexible server.
resource "azurerm_private_dns_zone" "postgres" {
  name                = "${var.name_prefix}.postgres.database.azure.com"
  resource_group_name = var.resource_group_name
  tags                = var.tags
}

resource "azurerm_private_dns_zone_virtual_network_link" "postgres" {
  name                  = "${var.name_prefix}-postgres-link"
  resource_group_name   = var.resource_group_name
  private_dns_zone_name = azurerm_private_dns_zone.postgres.name
  virtual_network_id    = azurerm_virtual_network.this.id
  tags                  = var.tags
}

# NSG with default-deny inbound from internet; AKS nodes get
# their own egress, intra-VNet traffic is allowed by default.
resource "azurerm_network_security_group" "aks" {
  name                = "${var.name_prefix}-aks-nsg"
  resource_group_name = var.resource_group_name
  location            = var.location
  tags                = var.tags

  security_rule {
    name                       = "DenyAllInboundFromInternet"
    priority                   = 4096
    direction                  = "Inbound"
    access                     = "Deny"
    protocol                   = "*"
    source_port_range          = "*"
    destination_port_range     = "*"
    source_address_prefix      = "Internet"
    destination_address_prefix = "*"
  }
}

resource "azurerm_subnet_network_security_group_association" "aks" {
  subnet_id                 = azurerm_subnet.aks.id
  network_security_group_id = azurerm_network_security_group.aks.id
}

# VNet Flow Logs via Network Watcher — required by SAS-SM gap
# analysis row "Network and infrastructure security".
data "azurerm_network_watcher" "this" {
  name                = "NetworkWatcher_${var.location}"
  resource_group_name = "NetworkWatcherRG"
}

resource "azurerm_storage_account" "flowlogs" {
  name                     = "${replace(var.name_prefix, "-", "")}flowlogs"
  resource_group_name      = var.resource_group_name
  location                 = var.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  min_tls_version          = "TLS1_2"
  tags                     = var.tags
}

resource "azurerm_network_watcher_flow_log" "this" {
  network_watcher_name = data.azurerm_network_watcher.this.name
  resource_group_name  = data.azurerm_network_watcher.this.resource_group_name
  name                 = "${var.name_prefix}-flowlog"

  target_resource_id = azurerm_network_security_group.aks.id
  storage_account_id = azurerm_storage_account.flowlogs.id
  enabled            = true
  version            = 2

  retention_policy {
    enabled = true
    days    = 365
  }

  traffic_analytics {
    enabled               = true
    workspace_id          = azurerm_log_analytics_workspace.this.workspace_id
    workspace_region      = azurerm_log_analytics_workspace.this.location
    workspace_resource_id = azurerm_log_analytics_workspace.this.id
    interval_in_minutes   = 10
  }

  tags = var.tags
}
