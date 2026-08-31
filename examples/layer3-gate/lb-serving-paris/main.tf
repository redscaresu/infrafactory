
resource "scaleway_instance_ip" "web" {
}

# python3 is already on the Ubuntu image, so the backend is serving within
# seconds of boot. Installing a package here would add a minute of apt to
# every gate run for no extra fidelity.
resource "scaleway_instance_server" "web" {
  name       = "if-s146-web"
  type       = "DEV1-S"
  image      = "ubuntu_jammy"
  ip_id      = scaleway_instance_ip.web.id

  user_data = {
    cloud-init = <<-EOT
      #!/bin/bash
      echo "infrafactory layer3 backend" > /root/index.html
      cd /root && nohup python3 -m http.server 80 >/dev/null 2>&1 &
    EOT
  }
}

resource "scaleway_lb_ip" "front" {
}

resource "scaleway_lb" "main" {
  name       = "if-s146-lb"
  ip_ids     = [scaleway_lb_ip.front.id]
  type       = "LB-S"
}

resource "scaleway_lb_backend" "web" {
  lb_id            = scaleway_lb.main.id
  name             = "web"
  forward_protocol = "http"
  forward_port     = 80
  server_ips       = [scaleway_instance_ip.web.address]
}

resource "scaleway_lb_frontend" "http" {
  lb_id        = scaleway_lb.main.id
  backend_id   = scaleway_lb_backend.web.id
  name         = "http"
  inbound_port = 80
}

output "lb_address" { value = scaleway_lb_ip.front.ip_address }
