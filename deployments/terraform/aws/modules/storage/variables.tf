variable "name_prefix" { type = string }
variable "primary_region" { type = string }
variable "replication_region" { type = string }
variable "retention_years" { type = number }
variable "audit_role_arn" { type = string }

variable "tags" {
  type    = map(string)
  default = {}
}
