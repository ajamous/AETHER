# Default provider — primary subscription.
provider "azurerm" {
  features {
    key_vault {
      # Soft-delete is mandatory on Azure; do NOT auto-purge on
      # destroy. Operators handle key-vault removal explicitly via
      # the SAS-SM key-ceremony procedure.
      purge_soft_delete_on_destroy    = false
      recover_soft_deleted_key_vaults = true
    }
    resource_group {
      prevent_deletion_if_contains_resources = true
    }
  }
}

