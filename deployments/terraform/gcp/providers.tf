# Default provider — primary region.
provider "google" {
  project = var.project_id
  region  = var.region
}
