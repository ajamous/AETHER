output "audit_sa_email" {
  value = google_service_account.audit.email
}

output "audit_sa_name" {
  value = google_service_account.audit.name
}

output "hsm_broker_sa_email" {
  value = google_service_account.hsm_broker.email
}

output "hsm_broker_sa_name" {
  value = google_service_account.hsm_broker.name
}
