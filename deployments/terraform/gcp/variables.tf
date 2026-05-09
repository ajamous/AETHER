# Top-level inputs for the Aether GCP reference deployment.
#
# Production posture (HSM protection level, Bucket Lock mode,
# private GKE, Flow Logs, CMEK, regional HA, backup retention) is
# deliberately not parameterised — those are policy, not preference.

variable "project_id" {
  description = "GCP project ID to deploy into."
  type        = string
}

variable "region" {
  description = "Primary GCP region. Pick from the GSMA-qualifying list (europe-west1, europe-west3, us-central1, etc.)."
  type        = string
  default     = "us-central1"
}

variable "name_prefix" {
  description = "Prefix for resource names. Keep short — GCP enforces length limits per resource type."
  type        = string
  default     = "aether"
}

variable "environment" {
  description = "Environment label (prod, staging, etc.). Applied as a label, not as part of the name."
  type        = string
  default     = "prod"
}

variable "labels" {
  description = "Additional labels merged into every resource."
  type        = map(string)
  default     = {}
}

# ---- Networking -------------------------------------------------------------

variable "vpc_cidr" {
  description = "Primary CIDR for the VPC subnet."
  type        = string
  default     = "10.10.0.0/20"
}

variable "pods_cidr" {
  description = "Secondary CIDR range for GKE pods."
  type        = string
  default     = "10.20.0.0/14"
}

variable "services_cidr" {
  description = "Secondary CIDR range for GKE services."
  type        = string
  default     = "10.24.0.0/20"
}

variable "master_cidr" {
  description = "RFC 1918 /28 reserved for the GKE control plane."
  type        = string
  default     = "172.16.0.0/28"
}

# ---- GKE --------------------------------------------------------------------

variable "gke_release_channel" {
  description = "GKE release channel."
  type        = string
  default     = "REGULAR"
}

variable "gke_master_authorized_cidrs" {
  description = "List of CIDRs allowed to reach the GKE control plane. Default is operator-VPN-only — adjust for your environment."
  type        = list(string)
  default     = []
}

# ---- Cloud SQL --------------------------------------------------------------

variable "cloudsql_tier" {
  description = "Cloud SQL machine tier. Reference doc calls db-perf-optimized-N-2 (4 vCPU 16GB) the floor."
  type        = string
  default     = "db-perf-optimized-N-2"
}

variable "cloudsql_disk_size_gb" {
  description = "Cloud SQL disk size."
  type        = number
  default     = 100
}

variable "cloudsql_backup_retention_days" {
  description = "Cloud SQL automated backup retention in days. Reference doc requires 35."
  type        = number
  default     = 35
}

# ---- Cloud KMS / HSM --------------------------------------------------------

variable "hsm_keyring_name" {
  description = "Cloud KMS key ring name for the HSM-protected signing keys."
  type        = string
  default     = "aether-hsm"
}

# ---- Storage ----------------------------------------------------------------

variable "audit_retention_years" {
  description = "Bucket Lock retention period in years for the audit-log bucket. SAS-SM Standard is typically 3 years minimum."
  type        = number
  default     = 3
}

variable "audit_bucket_dual_region" {
  description = "When true, create the audit bucket as dual-region (in-region redundancy plus reach to a second qualifying region)."
  type        = bool
  default     = true
}

variable "audit_bucket_secondary_region" {
  description = "Second region for dual-region audit bucket placement. Pick from the GSMA-qualifying list."
  type        = string
  default     = "europe-west3"
}
