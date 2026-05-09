resource "google_compute_network" "this" {
  name                    = "${var.name_prefix}-vpc"
  project                 = var.project_id
  auto_create_subnetworks = false
  routing_mode            = "REGIONAL"
}

resource "google_compute_subnetwork" "this" {
  name          = "${var.name_prefix}-subnet"
  project       = var.project_id
  region        = var.region
  network       = google_compute_network.this.self_link
  ip_cidr_range = var.vpc_cidr

  private_ip_google_access = true

  # VPC Flow Logs — required by the SAS-SM gap analysis row
  # "Network and infrastructure security".
  log_config {
    aggregation_interval = "INTERVAL_5_SEC"
    flow_sampling        = 0.5
    metadata             = "INCLUDE_ALL_METADATA"
  }

  secondary_ip_range {
    range_name    = "${var.name_prefix}-pods"
    ip_cidr_range = var.pods_cidr
  }

  secondary_ip_range {
    range_name    = "${var.name_prefix}-services"
    ip_cidr_range = var.services_cidr
  }
}

# Cloud Router + NAT — egress for private nodes.
resource "google_compute_router" "this" {
  name    = "${var.name_prefix}-router"
  project = var.project_id
  region  = var.region
  network = google_compute_network.this.self_link
}

resource "google_compute_router_nat" "this" {
  name                               = "${var.name_prefix}-nat"
  project                            = var.project_id
  region                             = var.region
  router                             = google_compute_router.this.name
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "ALL_SUBNETWORKS_ALL_IP_RANGES"

  log_config {
    enable = true
    filter = "ERRORS_ONLY"
  }
}

# Default-deny egress is operator policy; allow rules for
# private.googleapis.com, Cloud SQL, Cloud HSM, the audit bucket
# are added by the operator's own firewall stanza or via the
# `Allow Egress` button. The reference doc enumerates them.

resource "google_compute_firewall" "allow_internal" {
  name    = "${var.name_prefix}-allow-internal"
  project = var.project_id
  network = google_compute_network.this.self_link

  description = "Allow intra-VPC traffic between Aether workloads."
  direction   = "INGRESS"
  source_ranges = [
    var.vpc_cidr,
    var.pods_cidr,
    var.services_cidr,
  ]

  allow {
    protocol = "tcp"
  }
  allow {
    protocol = "udp"
  }
  allow {
    protocol = "icmp"
  }
}
