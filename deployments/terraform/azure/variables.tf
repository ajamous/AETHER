# Top-level inputs for the Aether Azure reference deployment.
#
# Production posture (Managed HSM, immutable storage, private
# AKS, Flow Logs, CMEK, zone-redundant HA, backup retention) is
# deliberately not parameterised — those are policy, not preference.

variable "location" {
  description = "Azure region. Pick from the GSMA-qualifying list (West Europe, North Europe, France Central, etc.). Managed HSM is only available in a subset of regions; verify before you build."
  type        = string
  default     = "westeurope"
}

variable "name_prefix" {
  description = "Prefix for resource names. Keep short — Azure enforces tight length and character-set limits per resource type."
  type        = string
  default     = "aether"
}

variable "environment" {
  description = "Environment label (prod, staging, etc.). Applied as a tag, not as part of the name."
  type        = string
  default     = "prod"
}

variable "tags" {
  description = "Additional tags merged into every resource."
  type        = map(string)
  default     = {}
}

# ---- Networking -------------------------------------------------------------

variable "vnet_cidr" {
  description = "Primary CIDR for the VNet."
  type        = string
  default     = "10.30.0.0/16"
}

variable "aks_subnet_cidr" {
  description = "Subnet CIDR for the AKS node pool."
  type        = string
  default     = "10.30.0.0/22"
}

variable "data_subnet_cidr" {
  description = "Subnet CIDR for managed services (Postgres flexible server, private endpoints)."
  type        = string
  default     = "10.30.4.0/24"
}

# ---- AKS --------------------------------------------------------------------

variable "aks_kubernetes_version" {
  description = "AKS Kubernetes version. Leave null to track Azure's default for the region."
  type        = string
  default     = null
}

variable "aks_node_vm_size" {
  description = "VM size for the system + workload node pool."
  type        = string
  default     = "Standard_D4s_v5"
}

variable "aks_node_count_min" {
  type    = number
  default = 3
}

variable "aks_node_count_max" {
  type    = number
  default = 6
}

variable "aks_authorized_ip_ranges" {
  description = "List of CIDRs allowed to reach the AKS API. Default is operator-VPN-only — adjust for your environment."
  type        = list(string)
  default     = []
}

# ---- Postgres ---------------------------------------------------------------

variable "postgres_sku_name" {
  description = "Azure Database for PostgreSQL Flexible Server SKU. Reference doc calls GP_Standard_D4ds_v5 (4 vCPU, 16 GiB) the floor."
  type        = string
  default     = "GP_Standard_D4ds_v5"
}

variable "postgres_storage_mb" {
  description = "Postgres disk size in MiB."
  type        = number
  default     = 131072 # 128 GiB
}

variable "postgres_version" {
  type    = string
  default = "16"
}

variable "postgres_backup_retention_days" {
  description = "Backup retention in days. Reference doc requires 35."
  type        = number
  default     = 35
}

# ---- Managed HSM ------------------------------------------------------------

variable "managed_hsm_admin_object_ids" {
  description = "List of AAD object IDs of the people/principals who will be Managed HSM administrators. The activation ceremony (security-domain download + restore) is manual; this list is who the ceremony grants."
  type        = list(string)
  default     = []
}

# ---- Storage ----------------------------------------------------------------

variable "audit_retention_years" {
  description = "Immutable container retention period in years for the audit-log storage. SAS-SM Standard is typically 3 years minimum."
  type        = number
  default     = 3
}
