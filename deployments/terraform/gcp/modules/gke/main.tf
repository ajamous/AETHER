# GKE Autopilot — the recommended starting point for an MVNO scale
# deployment per reference-gcp.md. Same security floor as Standard
# GKE, less node-management overhead. Operators who need a custom
# node pool can fork this submodule.
#
# Production posture:
#   - Private cluster (no public node IPs)
#   - Workload Identity enabled (the only way pods get GCP IAM)
#   - Cloud Logging + Cloud Monitoring enabled (audit-relevant)
#   - Release channel pinned via variable
#   - Master authorised networks must be supplied by the operator

resource "google_container_cluster" "this" {
  name     = "${var.name_prefix}-gke"
  project  = var.project_id
  location = var.region

  enable_autopilot = true

  network    = var.network
  subnetwork = var.subnetwork

  ip_allocation_policy {
    cluster_secondary_range_name  = var.pods_range_name
    services_secondary_range_name = var.services_range_name
  }

  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = false
    master_ipv4_cidr_block  = var.master_cidr
  }

  master_authorized_networks_config {
    dynamic "cidr_blocks" {
      for_each = var.master_authorized_cidrs
      content {
        cidr_block   = cidr_blocks.value
        display_name = "operator"
      }
    }
  }

  release_channel {
    channel = var.release_channel
  }

  workload_identity_config {
    workload_pool = "${var.project_id}.svc.id.goog"
  }

  logging_service    = "logging.googleapis.com/kubernetes"
  monitoring_service = "monitoring.googleapis.com/kubernetes"

  deletion_protection = true

  resource_labels = var.labels
}
