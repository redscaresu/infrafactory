terraform {
  required_version = ">= 1.6.0"

  required_providers {
    scaleway = {
      source = "scaleway/scaleway"
    }
  }
}

provider "scaleway" {
  region = var.region
  zone   = var.zone
}