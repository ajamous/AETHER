output "audit_bucket_name" {
  value = google_storage_bucket.audit.name
}

output "audit_bucket_url" {
  value = google_storage_bucket.audit.url
}

output "audit_kms_key_id" {
  value = google_kms_crypto_key.audit.id
}
