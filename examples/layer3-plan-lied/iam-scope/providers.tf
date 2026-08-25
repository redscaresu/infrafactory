terraform {
  required_providers {
    scaleway = { source = "scaleway/scaleway", version = "2.81.0" }
  }
}
provider "scaleway" {
  region = "fr-par"
  zone   = "fr-par-1"
}
