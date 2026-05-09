variable "name_prefix" { type = string }
variable "vpc_id" { type = string }
variable "subnet_ids" { type = list(string) }
variable "eks_node_security_group" { type = string }
variable "instance_class" { type = string }
variable "engine_version" { type = string }
variable "allocated_storage_gb" { type = number }
variable "backup_retention_days" { type = number }

variable "tags" {
  type    = map(string)
  default = {}
}
