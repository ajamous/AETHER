output "instance_name" {
  value = google_sql_database_instance.this.name
}

output "connection_name" {
  value = google_sql_database_instance.this.connection_name
}

output "private_ip_address" {
  value = google_sql_database_instance.this.private_ip_address
}

output "password_secret_id" {
  value = google_secret_manager_secret.aether.secret_id
}

output "password_secret_name" {
  value = google_secret_manager_secret.aether.name
}
