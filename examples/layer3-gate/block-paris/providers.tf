terraform {
  required_version = ">= 1.6.0"

  required_providers {
    scaleway = {
      source = "scaleway/scaleway"
      # Exact, not a range. The gate refuses anything else: this provider
      # executes with real cloud credentials, so which build runs is a
      # decision someone makes, not one the registry makes at init time.
      version = "2.81.0"
    }
  }
}

provider "scaleway" {
  region = var.region
  zone   = var.zone
}