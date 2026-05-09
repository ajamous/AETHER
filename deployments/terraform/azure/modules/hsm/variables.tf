variable "resource_group_name" {
  type = string
}

variable "location" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "tenant_id" {
  type = string
}

variable "admin_object_ids" {
  description = "Object IDs of the AAD principals who will be Managed HSM administrators after the security-domain ceremony. Empty means none — required for the create operation; activation ceremony is manual."
  type        = list(string)
}

variable "hsm_broker_principal_id" {
  description = "Object ID of the user-assigned managed identity the hsm-broker pod runs as."
  type        = string
}

variable "tags" {
  type    = map(string)
  default = {}
}
