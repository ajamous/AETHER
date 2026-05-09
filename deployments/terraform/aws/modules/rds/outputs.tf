output "endpoint" {
  value = aws_db_instance.this.endpoint
}

output "secret_arn" {
  description = "ARN of the Secrets Manager secret holding the RDS master password."
  value       = aws_secretsmanager_secret.master.arn
}

output "kms_key_arn" {
  value = aws_kms_key.rds.arn
}
