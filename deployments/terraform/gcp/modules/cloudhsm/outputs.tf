output "keyring_id" {
  value = google_kms_key_ring.hsm.id
}

output "keyring_name" {
  value = google_kms_key_ring.hsm.name
}
