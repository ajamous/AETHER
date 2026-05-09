output "cluster_id" {
  value = aws_cloudhsm_v2_cluster.this.cluster_id
}

output "security_group_id" {
  value = aws_security_group.this.id
}
