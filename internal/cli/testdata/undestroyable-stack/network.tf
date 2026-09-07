resource "scaleway_vpc" "main" {
  name   = "${var.scenario_name}-vpc"
  region = var.region
}

resource "scaleway_vpc_private_network" "main" {
  name   = "${var.scenario_name}-pn"
  vpc_id = scaleway_vpc.main.id
  region = var.region

  ipv4_subnet {
    subnet = var.private_network_subnet
  }
}