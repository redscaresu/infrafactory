variable "region" {
  description = "Scaleway region hosting the scenario resources"
  type        = string
  default     = "fr-par"
}

variable "zone" {
  description = "Scaleway availability zone hosting the block volume"
  type        = string
  default     = "fr-par-1"
}

variable "block_volume_name" {
  description = "Name of the app-data block volume"
  type        = string
  default     = "block-paris-app-data"
}

variable "block_volume_size_in_gb" {
  description = "Size of the app-data block volume in GB"
  type        = number
  default     = 10
}

variable "block_volume_iops" {
  description = "Maximum IO/s expected from the app-data block volume"
  type        = number
  default     = 5000
}

variable "tags" {
  description = "Tags applied to the app-data block volume"
  type        = list(string)
  default     = ["infrafactory", "block-paris", "app-data"]
}