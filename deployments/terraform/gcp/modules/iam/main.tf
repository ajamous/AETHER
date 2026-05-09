# GCP service accounts for the Aether services.
#
# Workload Identity binds each Kubernetes ServiceAccount the Helm
# chart creates (named `<release>-aether`) to the matching GCP
# service account. The actual binding is post-deploy because the
# GKE OIDC issuer + chart release name aren't known at apply time.
# See README §"Post-deploy Workload Identity wiring" for the
# `gcloud iam service-accounts add-iam-policy-binding ...` calls.

resource "google_service_account" "audit" {
  project      = var.project_id
  account_id   = "${var.name_prefix}-audit"
  display_name = "Aether audit service"
  description  = "Writes hash-chained audit log offsite copies to the WORM GCS bucket. Bound to the Kubernetes audit ServiceAccount via Workload Identity."
}

resource "google_service_account" "hsm_broker" {
  project      = var.project_id
  account_id   = "${var.name_prefix}-hsm-broker"
  display_name = "Aether HSM broker"
  description  = "Talks to Cloud HSM-protected keys via Cloud KMS. Bound to the Kubernetes hsm-broker ServiceAccount via Workload Identity."
}
