variable "resource_group_name" {
  type = string
}

variable "location" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "sku_name" {
  type = string
}

variable "storage_mb" {
  type = number
}

variable "postgres_version" {
  type = string
}

variable "backup_retention_days" {
  type = number
}

variable "delegated_subnet_id" {
  type = string
}

variable "private_dns_zone_id" {
  type = string
}

variable "tags" {
  type    = map(string)
  default = {}
}
