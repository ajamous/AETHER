variable "name_prefix" { type = string }
variable "cluster_version" { type = string }
variable "vpc_id" { type = string }
variable "subnet_ids" { type = list(string) }
variable "node_instance_type" { type = string }
variable "node_min_size" { type = number }
variable "node_desired_size" { type = number }
variable "node_max_size" { type = number }
variable "cluster_role_arn" { type = string }
variable "node_role_arn" { type = string }

variable "tags" {
  type    = map(string)
  default = {}
}
