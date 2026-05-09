data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  azs = slice(data.aws_availability_zones.available.names, 0, var.az_count)

  default_tags = merge(
    {
      "aether.environment" = var.environment
      "aether.managed-by"  = "terraform"
      "aether.module"      = "deployments/terraform/aws"
    },
    var.tags,
  )
}

module "network" {
  source = "./modules/network"

  name_prefix        = var.name_prefix
  vpc_cidr           = var.vpc_cidr
  availability_zones = local.azs
  tags               = local.default_tags
}

module "iam" {
  source = "./modules/iam"

  name_prefix = var.name_prefix
  tags        = local.default_tags
}

module "eks" {
  source = "./modules/eks"

  name_prefix        = var.name_prefix
  cluster_version    = var.eks_cluster_version
  vpc_id             = module.network.vpc_id
  subnet_ids         = module.network.private_subnet_ids
  node_instance_type = var.eks_node_instance_type
  node_min_size      = var.eks_node_min_size
  node_desired_size  = var.eks_node_desired_size
  node_max_size      = var.eks_node_max_size
  cluster_role_arn   = module.iam.eks_cluster_role_arn
  node_role_arn      = module.iam.eks_node_role_arn
  tags               = local.default_tags
}

module "rds" {
  source = "./modules/rds"

  name_prefix             = var.name_prefix
  vpc_id                  = module.network.vpc_id
  subnet_ids              = module.network.private_subnet_ids
  eks_node_security_group = module.eks.node_security_group_id
  instance_class          = var.rds_instance_class
  engine_version          = var.rds_engine_version
  allocated_storage_gb    = var.rds_allocated_storage_gb
  backup_retention_days   = var.rds_backup_retention_days
  tags                    = local.default_tags
}

module "cloudhsm" {
  source = "./modules/cloudhsm"
  count  = var.cloudhsm_enabled ? 1 : 0

  name_prefix = var.name_prefix
  vpc_id      = module.network.vpc_id
  subnet_ids  = module.network.private_subnet_ids
  hsm_count   = var.cloudhsm_count
  tags        = local.default_tags
}

module "storage" {
  source = "./modules/storage"

  name_prefix        = var.name_prefix
  primary_region     = var.region
  replication_region = var.audit_bucket_replication_region
  retention_years    = var.audit_retention_years
  audit_role_arn     = module.iam.audit_role_arn
  tags               = local.default_tags
}
