terraform {
  required_version = ">= 1.6.0"

  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "2.81.0"
    }
  }
}

provider "scaleway" {
  region = var.region
  zone   = var.zone
}