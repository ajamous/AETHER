# Default provider — primary region.
provider "aws" {
  region = var.region

  default_tags {
    tags = {
      "aether.environment" = var.environment
      "aether.managed-by"  = "terraform"
    }
  }
}
