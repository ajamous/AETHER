# Cloud HSM via Cloud KMS — HSM-protected keys.
#
# GCP "Cloud HSM" presents as a Cloud KMS key with
# protection_level = HSM, backed by FIPS 140-2 Level 3 hardware
# (Marvell LiquidSecurity adapter per the reference doc). The
# hsm-broker pod uses the Cloud KMS PKCS#11 library
# (libcloudkms.so) plus this key ring; no separate cluster ID to
# manage.
#
# Actual key generation (the SM-DP+ identity keys) is the manual
# two-person key-ceremony procedure documented in
# docs/sas-sm/key-ceremony.md. Terraform stops at the key ring
# and IAM bindings; humans do the ceremony.

resource "google_kms_key_ring" "hsm" {
  project  = var.project_id
  name     = var.keyring_name
  location = var.region
}

# IAM: hsm-broker SA can use HSM-protected keys to sign + verify.
resource "google_kms_key_ring_iam_member" "hsm_broker_signer" {
  key_ring_id = google_kms_key_ring.hsm.id
  role        = "roles/cloudkms.signerVerifier"
  member      = "serviceAccount:${var.hsm_broker_sa_email}"
}

# IAM: hsm-broker SA can list keys (so it can enumerate identity
# keys at startup).
resource "google_kms_key_ring_iam_member" "hsm_broker_viewer" {
  key_ring_id = google_kms_key_ring.hsm.id
  role        = "roles/cloudkms.viewer"
  member      = "serviceAccount:${var.hsm_broker_sa_email}"
}
