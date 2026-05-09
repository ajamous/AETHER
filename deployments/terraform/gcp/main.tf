locals {
  default_labels = merge(
    {
      "aether-environment" = var.environment
      "aether-managed-by"  = "terraform"
      "aether-module"      = "deployments-terraform-gcp"
    },
    var.labels,
  )
}

module "network" {
  source = "./modules/network"

  project_id    = var.project_id
  region        = var.region
  name_prefix   = var.name_prefix
  vpc_cidr      = var.vpc_cidr
  pods_cidr     = var.pods_cidr
  services_cidr = var.services_cidr
  labels        = local.default_labels
}

module "iam" {
  source = "./modules/iam"

  project_id  = var.project_id
  name_prefix = var.name_prefix
}

module "gke" {
  source = "./modules/gke"

  project_id              = var.project_id
  region                  = var.region
  name_prefix             = var.name_prefix
  network                 = module.network.network_self_link
  subnetwork              = module.network.subnetwork_self_link
  pods_range_name         = module.network.pods_range_name
  services_range_name     = module.network.services_range_name
  master_cidr             = var.master_cidr
  master_authorized_cidrs = var.gke_master_authorized_cidrs
  release_channel         = var.gke_release_channel
  labels                  = local.default_labels
}

module "cloudsql" {
  source = "./modules/cloudsql"

  project_id            = var.project_id
  region                = var.region
  name_prefix           = var.name_prefix
  network_self_link     = module.network.network_self_link
  tier                  = var.cloudsql_tier
  disk_size_gb          = var.cloudsql_disk_size_gb
  backup_retention_days = var.cloudsql_backup_retention_days
  labels                = local.default_labels
}

module "cloudhsm" {
  source = "./modules/cloudhsm"

  project_id              = var.project_id
  region                  = var.region
  keyring_name            = var.hsm_keyring_name
  hsm_broker_sa_email     = module.iam.hsm_broker_sa_email
  audit_sa_email          = module.iam.audit_sa_email
  audit_bucket_kms_unused = true # placeholder so dependency ordering is explicit
}

module "storage" {
  source = "./modules/storage"

  project_id       = var.project_id
  region           = var.region
  name_prefix      = var.name_prefix
  retention_years  = var.audit_retention_years
  dual_region      = var.audit_bucket_dual_region
  secondary_region = var.audit_bucket_secondary_region
  audit_sa_email   = module.iam.audit_sa_email
  labels           = local.default_labels
}
