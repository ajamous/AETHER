variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "name_prefix" {
  type = string
}

variable "network" {
  type = string
}

variable "subnetwork" {
  type = string
}

variable "pods_range_name" {
  type = string
}

variable "services_range_name" {
  type = string
}

variable "master_cidr" {
  type = string
}

variable "master_authorized_cidrs" {
  type    = list(string)
  default = []
}

variable "release_channel" {
  type    = string
  default = "REGULAR"
}

variable "labels" {
  type    = map(string)
  default = {}
}
