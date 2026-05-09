# Audit-log offsite bucket with Bucket Lock retention policy in
# Compliance mode + CMEK + lifecycle to Coldline after 90 days.
#
# Bucket Lock Compliance: once locked, even the project owner
# cannot shorten the retention period. Like AWS S3 Object Lock
# Compliance.

resource "random_id" "bucket_suffix" {
  byte_length = 4
}

resource "google_kms_key_ring" "audit" {
  project  = var.project_id
  name     = "${var.name_prefix}-audit"
  location = var.region
}

resource "google_kms_crypto_key" "audit" {
  name            = "${var.name_prefix}-audit-key"
  key_ring        = google_kms_key_ring.audit.id
  rotation_period = "7776000s" # 90 days
}

# GCS service agent must be allowed to use the CMEK on the bucket.
data "google_storage_project_service_account" "this" {
  project = var.project_id
}

resource "google_kms_crypto_key_iam_member" "gcs_sa" {
  crypto_key_id = google_kms_crypto_key.audit.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = "serviceAccount:${data.google_storage_project_service_account.this.email_address}"
}

resource "google_storage_bucket" "audit" {
  project = var.project_id
  name    = "${var.name_prefix}-audit-${random_id.bucket_suffix.hex}"

  # Dual-region placement when the operator wants in-region
  # redundancy plus reach to a second qualifying region.
  location = var.dual_region ? upper(var.region) : var.region

  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"

  versioning {
    enabled = true
  }

  encryption {
    default_kms_key_name = google_kms_crypto_key.audit.id
  }

  retention_policy {
    retention_period = var.retention_years * 365 * 24 * 60 * 60
    is_locked        = true
  }

  lifecycle_rule {
    action {
      type          = "SetStorageClass"
      storage_class = "COLDLINE"
    }
    condition {
      age = 90
    }
  }

  labels = var.labels

  # Bucket Lock cannot be removed; deletion_protection is implicit.
  depends_on = [google_kms_crypto_key_iam_member.gcs_sa]
}

# Audit service account writes objects only — no delete, no
# retention shortening (Bucket Lock would block it anyway).
resource "google_storage_bucket_iam_member" "audit_writer" {
  bucket = google_storage_bucket.audit.name
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${var.audit_sa_email}"
}

# Audit service account also needs to use the CMEK to encrypt
# uploads.
resource "google_kms_crypto_key_iam_member" "audit_sa_encrypter" {
  crypto_key_id = google_kms_crypto_key.audit.id
  role          = "roles/cloudkms.cryptoKeyEncrypter"
  member        = "serviceAccount:${var.audit_sa_email}"
}
