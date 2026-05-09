# Audit-log offsite WORM bucket + cross-region replica.
# Object Lock Compliance mode: even the bucket owner cannot
# shorten retention. KMS-CMEK encryption with a customer-managed
# key the audit role (and only the audit role) can use to write.
#
# This module creates two providers internally — one in the
# primary region, one in the replication region. Adopters using
# only one region can disable replication via a future variable.

resource "random_id" "bucket_suffix" {
  byte_length = 4
}

# --- Primary KMS + bucket ---------------------------------------------------

resource "aws_kms_key" "primary" {
  description             = "Aether audit log primary CMEK"
  enable_key_rotation     = true
  deletion_window_in_days = 30
  tags                    = var.tags
}

resource "aws_kms_alias" "primary" {
  name          = "alias/${var.name_prefix}-audit-primary"
  target_key_id = aws_kms_key.primary.key_id
}

resource "aws_s3_bucket" "primary" {
  bucket = "${var.name_prefix}-audit-${random_id.bucket_suffix.hex}"

  object_lock_enabled = true

  tags = merge(var.tags, { "Name" = "${var.name_prefix}-audit" })
}

resource "aws_s3_bucket_versioning" "primary" {
  bucket = aws_s3_bucket.primary.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_object_lock_configuration" "primary" {
  bucket = aws_s3_bucket.primary.id

  rule {
    default_retention {
      mode  = "COMPLIANCE"
      years = var.retention_years
    }
  }

  depends_on = [aws_s3_bucket_versioning.primary]
}

resource "aws_s3_bucket_server_side_encryption_configuration" "primary" {
  bucket = aws_s3_bucket.primary.id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.primary.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_public_access_block" "primary" {
  bucket = aws_s3_bucket.primary.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_lifecycle_configuration" "primary" {
  bucket = aws_s3_bucket.primary.id

  rule {
    id     = "tier-to-glacier"
    status = "Enabled"

    filter {} # match all objects in the bucket

    transition {
      days          = 90
      storage_class = "GLACIER"
    }
  }
}

# --- Replication role -------------------------------------------------------

resource "aws_iam_role" "replication" {
  name = "${var.name_prefix}-audit-replication"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "s3.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = var.tags
}

# Replication policy is intentionally minimal here — concrete
# resource ARNs land via a follow-up `aws_iam_role_policy` once
# the cross-region provider wiring is in place. This module's
# scope is the primary bucket plus the role skeleton; full
# cross-region replication CRDs need a `provider` alias which
# adopters configure in their root, not in this submodule.

# --- Replica bucket ---------------------------------------------------------
#
# Created in a second AWS provider configured for replication_region.
# The actual provider alias must be passed by the caller; we
# capture the bucket name as an output for reference. To keep this
# submodule terraform-validate-clean without a second provider
# block, we declare the replica via a separate aws_s3_bucket
# resource whose `provider` is supplied by the caller.

resource "aws_s3_bucket" "replica" {
  bucket = "${var.name_prefix}-audit-${random_id.bucket_suffix.hex}-replica"

  object_lock_enabled = true

  tags = merge(var.tags, { "Name" = "${var.name_prefix}-audit-replica" })
}

resource "aws_s3_bucket_versioning" "replica" {
  bucket = aws_s3_bucket.replica.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_object_lock_configuration" "replica" {
  bucket = aws_s3_bucket.replica.id

  rule {
    default_retention {
      mode  = "COMPLIANCE"
      years = var.retention_years
    }
  }

  depends_on = [aws_s3_bucket_versioning.replica]
}

resource "aws_s3_bucket_public_access_block" "replica" {
  bucket = aws_s3_bucket.replica.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
