resource "aws_db_subnet_group" "this" {
  name       = "${var.name_prefix}-rds"
  subnet_ids = var.subnet_ids
  tags       = var.tags
}

resource "aws_security_group" "this" {
  name        = "${var.name_prefix}-rds"
  description = "Allow Postgres from EKS nodes only"
  vpc_id      = var.vpc_id

  ingress {
    description     = "Postgres from EKS"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [var.eks_node_security_group]
  }

  egress {
    description = "Outbound: DB initiates no calls"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = var.tags
}

resource "aws_kms_key" "rds" {
  description             = "Aether RDS storage encryption key"
  enable_key_rotation     = true
  deletion_window_in_days = 30
  tags                    = var.tags
}

resource "aws_kms_alias" "rds" {
  name          = "alias/${var.name_prefix}-rds"
  target_key_id = aws_kms_key.rds.key_id
}

resource "random_password" "master" {
  length           = 32
  special          = true
  min_special      = 4
  override_special = "!#$%&*-_=+[]{}<>?"
}

resource "aws_secretsmanager_secret" "master" {
  name        = "${var.name_prefix}/rds/master-password"
  description = "Aether RDS master password"
  kms_key_id  = aws_kms_key.rds.arn
  tags        = var.tags
}

resource "aws_secretsmanager_secret_version" "master" {
  secret_id     = aws_secretsmanager_secret.master.id
  secret_string = random_password.master.result
}

resource "aws_db_instance" "this" {
  identifier                = "${var.name_prefix}-rds"
  engine                    = "postgres"
  engine_version            = var.engine_version
  instance_class            = var.instance_class
  allocated_storage         = var.allocated_storage_gb
  storage_type              = "gp3"
  storage_encrypted         = true
  kms_key_id                = aws_kms_key.rds.arn
  db_name                   = "aether"
  username                  = "aether"
  password                  = random_password.master.result
  multi_az                  = true
  publicly_accessible       = false
  db_subnet_group_name      = aws_db_subnet_group.this.name
  vpc_security_group_ids    = [aws_security_group.this.id]
  backup_retention_period   = var.backup_retention_days
  backup_window             = "03:00-05:00"
  maintenance_window        = "sun:05:00-sun:07:00"
  copy_tags_to_snapshot     = true
  deletion_protection       = true
  skip_final_snapshot       = false
  final_snapshot_identifier = "${var.name_prefix}-rds-final"

  enabled_cloudwatch_logs_exports = ["postgresql"]

  performance_insights_enabled          = true
  performance_insights_retention_period = 31
  performance_insights_kms_key_id       = aws_kms_key.rds.arn

  tags = var.tags
}
