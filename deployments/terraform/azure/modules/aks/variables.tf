variable "resource_group_name" {
  type = string
}

variable "location" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "kubernetes_version" {
  type = string
}

variable "node_vm_size" {
  type = string
}

variable "node_count_min" {
  type = number
}

variable "node_count_max" {
  type = number
}

variable "aks_subnet_id" {
  type = string
}

variable "authorized_ip_ranges" {
  type    = list(string)
  default = []
}

variable "log_analytics_workspace_id" {
  type = string
}

variable "tags" {
  type    = map(string)
  default = {}
}
