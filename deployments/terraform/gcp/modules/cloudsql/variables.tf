variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "network_self_link" {
  type = string
}

variable "tier" {
  type = string
}

variable "disk_size_gb" {
  type = number
}

variable "backup_retention_days" {
  type = number
}

variable "labels" {
  type    = map(string)
  default = {}
}
