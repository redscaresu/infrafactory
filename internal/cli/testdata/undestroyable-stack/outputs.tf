output "vpc_id" {
  description = "ID of the VPC."
  value       = scaleway_vpc.main.id
}

output "private_network_id" {
  description = "ID of the private network shared by the instance and the load balancer."
  value       = scaleway_vpc_private_network.main.id
}

output "instance_id" {
  description = "ID of the web instance."
  value       = scaleway_instance_server.web.id
}

output "instance_public_ip" {
  description = "Public IPv4 of the web instance, used for egress to pull the container image."
  value       = scaleway_instance_ip.web.address
}

output "instance_private_ip" {
  description = "Private IPv4 of the web instance on the private network."
  value       = scaleway_instance_private_nic.web.private_ips[0].address
}

output "load_balancer_id" {
  description = "ID of the load balancer."
  value       = scaleway_lb.main.id
}

output "load_balancer_ip" {
  description = "Public IPv4 of the load balancer."
  value       = scaleway_lb_ip.main.ip_address
}

output "load_balancer_endpoint" {
  description = "HTTP endpoint the probe polls."
  value       = "http://${scaleway_lb_ip.main.ip_address}:${var.lb_inbound_port}${var.service_health_path}"
}

output "service_version" {
  description = "The pinned application image running on the instance."
  value       = "${var.service_image}:${var.service_tag}"
}