output "primary_bucket_name" {
  value = aws_s3_bucket.primary.id
}

output "replica_bucket_name" {
  value = aws_s3_bucket.replica.id
}

output "primary_kms_key_arn" {
  value = aws_kms_key.primary.arn
}

output "replication_role_arn" {
  value = aws_iam_role.replication.arn
}
