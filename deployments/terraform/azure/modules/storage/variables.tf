variable "resource_group_name" {
  type = string
}

variable "location" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "retention_years" {
  type = number
}

variable "audit_principal_id" {
  type = string
}

variable "tags" {
  type    = map(string)
  default = {}
}
