# Per-service IAM roles. The chart binds these to the per-service
# Kubernetes ServiceAccounts via IRSA (IAM Roles for Service Accounts).
# Each service gets only the AWS permissions it needs.
#
# The IRSA trust policies here are skeletal — the actual OIDC
# provider ARN comes from the EKS cluster, which is created by a
# sibling module. Adopters wiring this end-to-end either:
#   1. Run terraform twice (first the EKS cluster, then this with
#      the OIDC provider ARN passed in), or
#   2. Use a separate post-deploy step to attach the OIDC trust.
# For the reference deployment, option 2 is documented in
# docs/sas-sm/reference-aws.md.

# EKS cluster role
resource "aws_iam_role" "eks_cluster" {
  name = "${var.name_prefix}-eks-cluster"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "eks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = var.tags
}

resource "aws_iam_role_policy_attachment" "eks_cluster" {
  for_each = toset([
    "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy",
    "arn:aws:iam::aws:policy/AmazonEKSVPCResourceController",
  ])
  role       = aws_iam_role.eks_cluster.name
  policy_arn = each.value
}

# EKS node role
resource "aws_iam_role" "eks_node" {
  name = "${var.name_prefix}-eks-node"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = var.tags
}

resource "aws_iam_role_policy_attachment" "eks_node" {
  for_each = toset([
    "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
    "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
    "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
  ])
  role       = aws_iam_role.eks_node.name
  policy_arn = each.value
}

# Audit service IRSA role — writes to the WORM bucket only.
resource "aws_iam_role" "audit" {
  name = "${var.name_prefix}-audit"

  # Trust policy is a placeholder; bind to the EKS OIDC provider
  # post-deploy. See module README §"Post-deploy IRSA wiring."
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = var.tags
}

# HSM broker IRSA role — talks to CloudHSM.
resource "aws_iam_role" "hsm_broker" {
  name = "${var.name_prefix}-hsm-broker"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })

  tags = var.tags
}

resource "aws_iam_role_policy" "hsm_broker" {
  name = "${var.name_prefix}-hsm-broker-cloudhsm"
  role = aws_iam_role.hsm_broker.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "cloudhsm:DescribeClusters",
        "cloudhsm:DescribeBackups",
      ]
      Resource = "*"
    }]
  })
}
