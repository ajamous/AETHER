variable "name_prefix" { type = string }
variable "vpc_id" { type = string }
variable "subnet_ids" { type = list(string) }
variable "hsm_count" { type = number }

variable "tags" {
  type    = map(string)
  default = {}
}
