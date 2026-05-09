output "eks_cluster_role_arn" {
  value = aws_iam_role.eks_cluster.arn
}

output "eks_node_role_arn" {
  value = aws_iam_role.eks_node.arn
}

output "audit_role_arn" {
  value = aws_iam_role.audit.arn
}

output "hsm_broker_role_arn" {
  value = aws_iam_role.hsm_broker.arn
}
