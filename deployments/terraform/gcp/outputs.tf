output "vpc_self_link" {
  description = "VPC the Aether deployment lives in."
  value       = module.network.network_self_link
}

output "subnetwork_self_link" {
  description = "Primary subnet for Aether workloads."
  value       = module.network.subnetwork_self_link
}

output "gke_cluster_name" {
  description = "GKE cluster name. Use to authenticate kubectl: `gcloud container clusters get-credentials <this>`."
  value       = module.gke.cluster_name
}

output "gke_cluster_endpoint" {
  description = "GKE control plane endpoint."
  value       = module.gke.cluster_endpoint
  sensitive   = true
}

output "cloudsql_instance_name" {
  description = "Cloud SQL instance name. Goes into the chart's postgresUrl via the Cloud SQL Auth Proxy."
  value       = module.cloudsql.instance_name
}

output "cloudsql_connection_name" {
  description = "Cloud SQL connection name (project:region:instance) — the value the Auth Proxy needs."
  value       = module.cloudsql.connection_name
}

output "cloudsql_password_secret_id" {
  description = "Secret Manager secret ID holding the Cloud SQL aether-user password."
  value       = module.cloudsql.password_secret_id
}

output "audit_bucket_name" {
  description = "WORM audit-log bucket. Bucket Lock Compliance mode."
  value       = module.storage.audit_bucket_name
}

output "audit_kms_key_id" {
  description = "CMEK protecting the audit bucket."
  value       = module.storage.audit_kms_key_id
}

output "hsm_keyring_id" {
  description = "Cloud KMS key ring (HSM protection level) for Aether signing keys."
  value       = module.cloudhsm.keyring_id
}

output "audit_sa_email" {
  description = "Service account the audit pod runs as via Workload Identity."
  value       = module.iam.audit_sa_email
}

output "hsm_broker_sa_email" {
  description = "Service account the hsm-broker pod runs as via Workload Identity."
  value       = module.iam.hsm_broker_sa_email
}
