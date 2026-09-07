resource "scaleway_lb_ip" "main" {
  zone = var.zone
}

resource "scaleway_lb" "main" {
  name   = "${var.scenario_name}-lb"
  type   = var.lb_type
  ip_ids = [scaleway_lb_ip.main.id]
  zone   = var.zone

  private_network {
    private_network_id = scaleway_vpc_private_network.main.id
  }
}

resource "scaleway_lb_backend" "main" {
  name             = "http-backend"
  lb_id            = scaleway_lb.main.id
  forward_protocol = "http"
  forward_port     = var.service_port

  server_ips = [
    scaleway_instance_private_nic.web.private_ips[0].address,
  ]

  health_check_port        = var.service_port
  health_check_delay       = "10s"
  health_check_timeout     = "5s"
  health_check_max_retries = 5

  health_check_http {
    uri    = var.service_health_path
    method = "GET"
    code   = 200
  }
}

resource "scaleway_lb_frontend" "main" {
  name         = "http-frontend"
  lb_id        = scaleway_lb.main.id
  backend_id   = scaleway_lb_backend.main.id
  inbound_port = var.lb_inbound_port
}