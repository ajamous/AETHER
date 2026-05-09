# Top-level inputs for the Aether AWS reference deployment.
#
# Defaults reflect docs/sas-sm/reference-aws.md. The module is
# parameterised so adopters can change region, sizing, and naming
# without forking; SAS-SM-relevant defaults (encryption, retention,
# logging) are not parameterised — those are policy, not preference.

variable "region" {
  description = "AWS region. Must be a GSMA-qualifying region for SAS-SM accreditation. Confirm with your auditor."
  type        = string
  default     = "us-east-2"
}

variable "name_prefix" {
  description = "Prefix used on every resource name; lets multiple Aether deployments coexist in one account."
  type        = string
  default     = "aether"

  validation {
    condition     = can(regex("^[a-z0-9-]{1,32}$", var.name_prefix))
    error_message = "name_prefix must be 1-32 chars, lowercase alphanumeric or hyphen."
  }
}

variable "environment" {
  description = "Environment label (prod, staging, etc.) — applied as a tag, not as part of the name."
  type        = string
  default     = "prod"
}

variable "tags" {
  description = "Additional tags applied to every resource."
  type        = map(string)
  default     = {}
}

# --- Network ----------------------------------------------------------------

variable "vpc_cidr" {
  description = "CIDR block for the Aether VPC."
  type        = string
  default     = "10.40.0.0/16"
}

variable "az_count" {
  description = "Number of Availability Zones to spread across. SAS-SM baseline is 3."
  type        = number
  default     = 3

  validation {
    condition     = var.az_count >= 2 && var.az_count <= 6
    error_message = "az_count must be between 2 and 6."
  }
}

# --- EKS --------------------------------------------------------------------

variable "eks_cluster_version" {
  description = "Kubernetes version for the EKS control plane."
  type        = string
  default     = "1.31"
}

variable "eks_node_instance_type" {
  description = "EC2 instance type for the EKS managed node group. m6i.xlarge is the documented baseline."
  type        = string
  default     = "m6i.xlarge"
}

variable "eks_node_min_size" {
  description = "Minimum number of EKS worker nodes."
  type        = number
  default     = 3
}

variable "eks_node_desired_size" {
  description = "Desired number of EKS worker nodes."
  type        = number
  default     = 3
}

variable "eks_node_max_size" {
  description = "Maximum number of EKS worker nodes for autoscaling."
  type        = number
  default     = 6
}

# --- RDS --------------------------------------------------------------------

variable "rds_instance_class" {
  description = "RDS instance class. db.m6g.large is the documented baseline."
  type        = string
  default     = "db.m6g.large"
}

variable "rds_engine_version" {
  description = "Postgres engine version."
  type        = string
  default     = "16.4"
}

variable "rds_allocated_storage_gb" {
  description = "RDS allocated storage in GB."
  type        = number
  default     = 100
}

variable "rds_backup_retention_days" {
  description = "RDS automated backup retention. Reference deployment specifies 35 days."
  type        = number
  default     = 35
}

# --- CloudHSM ---------------------------------------------------------------

variable "cloudhsm_enabled" {
  description = "Whether to provision the CloudHSM cluster. Set false if you bring your own HSM and want only the rest of the deployment."
  type        = bool
  default     = true
}

variable "cloudhsm_count" {
  description = "Number of HSMs in the CloudHSM cluster. SAS-SM HA baseline is 2."
  type        = number
  default     = 2

  validation {
    condition     = var.cloudhsm_count >= 1 && var.cloudhsm_count <= 4
    error_message = "cloudhsm_count must be between 1 and 4."
  }
}

# --- Audit log offsite ------------------------------------------------------

variable "audit_bucket_replication_region" {
  description = "Region for the audit-log replication bucket. Must also be a GSMA-qualifying region."
  type        = string
  default     = "eu-west-3"
}

variable "audit_retention_years" {
  description = "Object Lock Compliance retention in years. SAS-SM baseline is 3."
  type        = number
  default     = 3
}
