# CloudHSM cluster — FIPS 140-2 Level 3 partition.
# Each HSM lives in a different AZ for cross-AZ HA.
#
# Cluster activation (initial admin PIN ceremony) is intentionally
# OUT of scope for this module: it is a manual operator step that
# must be performed under the docs/sas-sm/key-ceremony.md
# procedure with two-person quorum and a signed chain-of-custody
# form. Terraform brings the cluster up; humans take it from there.

resource "aws_security_group" "this" {
  name        = "${var.name_prefix}-cloudhsm"
  description = "CloudHSM client port from EKS nodes"
  vpc_id      = var.vpc_id

  ingress {
    description = "HSM client traffic from VPC"
    from_port   = 2223
    to_port     = 2225
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/8"]
  }

  egress {
    description = "Outbound default deny would break replication; default allow"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = var.tags
}

resource "aws_cloudhsm_v2_cluster" "this" {
  hsm_type   = "hsm1.medium"
  subnet_ids = var.subnet_ids

  tags = merge(var.tags, { "Name" = "${var.name_prefix}-cloudhsm" })
}

resource "aws_cloudhsm_v2_hsm" "this" {
  count      = var.hsm_count
  cluster_id = aws_cloudhsm_v2_cluster.this.cluster_id
  subnet_id  = var.subnet_ids[count.index % length(var.subnet_ids)]
}
