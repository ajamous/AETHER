# Cloud SQL Postgres 16 — regional HA, CMEK, PITR, 35-day backups.

resource "google_kms_key_ring" "cloudsql" {
  project  = var.project_id
  name     = "${var.name_prefix}-cloudsql"
  location = var.region
}

resource "google_kms_crypto_key" "cloudsql" {
  name            = "${var.name_prefix}-cloudsql-key"
  key_ring        = google_kms_key_ring.cloudsql.id
  rotation_period = "7776000s" # 90 days

  lifecycle {
    prevent_destroy = false
  }
}

# Grant the Cloud SQL service agent permission to use the CMEK.
data "google_project" "this" {
  project_id = var.project_id
}

resource "google_kms_crypto_key_iam_member" "cloudsql_sa" {
  crypto_key_id = google_kms_crypto_key.cloudsql.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = "serviceAccount:service-${data.google_project.this.number}@gcp-sa-cloud-sql.iam.gserviceaccount.com"
}

resource "random_password" "aether" {
  length      = 32
  special     = true
  min_special = 4
  # Cloud SQL Postgres rejects some specials in passwords passed
  # via the API; restrict to a safe set.
  override_special = "!#$%&*-_=+[]{}<>?"
}

resource "google_secret_manager_secret" "aether" {
  project   = var.project_id
  secret_id = "${var.name_prefix}-cloudsql-aether-password"

  replication {
    auto {}
  }

  labels = var.labels
}

resource "google_secret_manager_secret_version" "aether" {
  secret      = google_secret_manager_secret.aether.id
  secret_data = random_password.aether.result
}

resource "google_sql_database_instance" "this" {
  project          = var.project_id
  name             = "${var.name_prefix}-pg"
  region           = var.region
  database_version = "POSTGRES_16"

  encryption_key_name = google_kms_crypto_key.cloudsql.id

  deletion_protection = true

  settings {
    tier              = var.tier
    availability_type = "REGIONAL"
    disk_size         = var.disk_size_gb
    disk_type         = "PD_SSD"
    disk_autoresize   = true

    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
      backup_retention_settings {
        retained_backups = var.backup_retention_days
        retention_unit   = "COUNT"
      }
      transaction_log_retention_days = 7
    }

    ip_configuration {
      ipv4_enabled    = false
      private_network = var.network_self_link
      ssl_mode        = "ENCRYPTED_ONLY"
    }

    database_flags {
      name  = "cloudsql.iam_authentication"
      value = "on"
    }

    insights_config {
      query_insights_enabled  = true
      query_string_length     = 1024
      record_application_tags = true
      record_client_address   = false
    }

    user_labels = var.labels
  }

  depends_on = [google_kms_crypto_key_iam_member.cloudsql_sa]
}

resource "google_sql_database" "aether" {
  project  = var.project_id
  name     = "aether"
  instance = google_sql_database_instance.this.name
}

resource "google_sql_user" "aether" {
  project  = var.project_id
  name     = "aether"
  instance = google_sql_database_instance.this.name
  password = random_password.aether.result
}
