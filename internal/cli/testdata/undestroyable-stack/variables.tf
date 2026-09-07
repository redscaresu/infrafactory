variable "region" {
  description = "Scaleway region for regional resources."
  type        = string
  default     = "fr-par"
}

variable "zone" {
  description = "Scaleway zone for zonal resources."
  type        = string
  default     = "fr-par-1"
}

variable "scenario_name" {
  description = "Prefix applied to every resource name in this scenario."
  type        = string
  default     = "web-live-paris"
}

variable "private_network_subnet" {
  description = "IPv4 CIDR of the private network the instance and load balancer share."
  type        = string
  default     = "10.0.0.0/24"
}

variable "instance_type" {
  description = "Commercial type of the web instance."
  type        = string
  default     = "DEV1-S"
}

variable "instance_image" {
  description = "Base image label for the web instance."
  type        = string
  default     = "ubuntu_jammy"
}

variable "lb_type" {
  description = "Commercial type of the load balancer."
  type        = string
  default     = "LB-S"
}

variable "service_image" {
  description = "Container image of the application, without tag."
  type        = string
  default     = "nginx"
}

variable "service_tag" {
  description = "Immutable tag of the application image. Never latest."
  type        = string
  default     = "1.27"
}

variable "service_port" {
  description = "Port the container publishes on the host and the load balancer forwards to."
  type        = number
  default     = 80
}

variable "service_health_path" {
  description = "HTTP path the load balancer health check polls."
  type        = string
  default     = "/"
}

variable "lb_inbound_port" {
  description = "Public TCP port the load balancer listens on."
  type        = number
  default     = 80
}