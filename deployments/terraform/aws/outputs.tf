output "vpc_id" {
  description = "VPC the Aether deployment lives in."
  value       = module.network.vpc_id
}

output "eks_cluster_name" {
  description = "EKS cluster name. Use to authenticate kubectl: `aws eks update-kubeconfig --name <this>`."
  value       = module.eks.cluster_name
}

output "eks_cluster_endpoint" {
  description = "EKS cluster API server endpoint."
  value       = module.eks.cluster_endpoint
}

output "rds_endpoint" {
  description = "RDS Postgres endpoint. Goes into the Helm chart's postgresUrl."
  value       = module.rds.endpoint
}

output "rds_secret_arn" {
  description = "ARN of the Secrets Manager secret holding the RDS master password."
  value       = module.rds.secret_arn
}

output "audit_bucket_name" {
  description = "Primary WORM bucket for audit-log offsite copies."
  value       = module.storage.primary_bucket_name
}

output "audit_replica_bucket_name" {
  description = "Cross-region replica WORM bucket."
  value       = module.storage.replica_bucket_name
}

output "cloudhsm_cluster_id" {
  description = "CloudHSM cluster ID. Empty when cloudhsm_enabled = false."
  value       = var.cloudhsm_enabled ? module.cloudhsm[0].cluster_id : ""
}

output "audit_role_arn" {
  description = "IAM role for the audit service to write to the WORM bucket (bind via IRSA)."
  value       = module.iam.audit_role_arn
}

output "hsm_broker_role_arn" {
  description = "IAM role for hsm-broker to talk to CloudHSM (bind via IRSA)."
  value       = module.iam.hsm_broker_role_arn
}
