variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "retention_years" {
  type = number
}

variable "dual_region" {
  type    = bool
  default = true
}

variable "secondary_region" {
  type = string
}

variable "audit_sa_email" {
  type = string
}

variable "labels" {
  type    = map(string)
  default = {}
}
