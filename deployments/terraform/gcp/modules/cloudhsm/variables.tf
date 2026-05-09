variable "project_id" {
  type = string
}

variable "region" {
  type = string
}

variable "keyring_name" {
  type = string
}

variable "hsm_broker_sa_email" {
  type = string
}

variable "audit_sa_email" {
  type = string
}

# Placeholder so the wiring in the root module makes data
# dependencies explicit; not used inside this module.
variable "audit_bucket_kms_unused" {
  type    = bool
  default = true
}
