# What network.tf needs to plan: the provider pin and the VPC it names.

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

resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
