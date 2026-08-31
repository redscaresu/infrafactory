variable "region" {
  description = "Scaleway region for the block-paris scenario"
  type        = string
  default     = "fr-par"
}

variable "zone" {
  description = "Scaleway availability zone for the block-paris scenario"
  type        = string
  default     = "fr-par-1"
}

variable "project_name" {
  description = "Name of the bootstrapped Scaleway project"
  type        = string
  default     = "infrafactory-block-paris"
}

variable "project_description" {
  description = "Description of the bootstrapped Scaleway project"
  type        = string
  default     = "Bootstrapped project for the block-paris scenario"
}

variable "volume_name" {
  description = "Name of the application data block volume"
  type        = string
  default     = "block-paris-app-data"
}

variable "volume_size_in_gb" {
  description = "Size of the application data block volume in GB"
  type        = number
  default     = 10
}

variable "volume_iops" {
  description = "Maximum IO/s expected for the application data block volume"
  type        = number
  default     = 5000
}

variable "volume_tags" {
  description = "Tags applied to the application data block volume"
  type        = list(string)
  default     = ["infrafactory", "block-paris", "app-data"]
}