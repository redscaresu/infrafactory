resource "scaleway_instance_ip" "web" {
  zone = var.zone
}

resource "scaleway_instance_server" "web" {
  name  = "${var.scenario_name}-web"
  type  = var.instance_type
  image = var.instance_image
  ip_id = scaleway_instance_ip.web.id
  zone  = var.zone

  root_volume {
    delete_on_termination = true
  }

  user_data = {
    "cloud-init" = <<-EOT
      #!/bin/bash
      set -eux
      export DEBIAN_FRONTEND=noninteractive
      apt-get update
      apt-get install -y docker.io
      systemctl enable --now docker
      for i in $(seq 1 60); do
        docker info >/dev/null 2>&1 && break
        sleep 5
      done
      docker info >/dev/null 2>&1
      docker run -d --name web --restart=always \
        -p ${var.service_port}:${var.service_port} \
        ${var.service_image}:${var.service_tag}
    EOT
  }
}

resource "scaleway_instance_private_nic" "web" {
  server_id          = scaleway_instance_server.web.id
  private_network_id = scaleway_vpc_private_network.main.id
  zone               = var.zone
}