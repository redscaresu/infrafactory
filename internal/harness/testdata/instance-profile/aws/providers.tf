# Copied into every capture workdir by capture.sh.
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "5.100.0"
    }
  }
}

provider "aws" {
  region              = "us-east-1"
  allowed_account_ids = ["000000000000"]
}
