


output "block_volume_id" {
  description = "ID of the application data block volume"
  value       = scaleway_block_volume.app_data.id
}

output "block_volume_name" {
  description = "Name of the application data block volume"
  value       = scaleway_block_volume.app_data.name
}

output "block_volume_srn" {
  description = "Scaleway Resource Name (SRN) of the application data block volume"
  value       = scaleway_block_volume.app_data.srn
}

output "block_volume_zone" {
  description = "Zone the application data block volume is attached to"
  value       = scaleway_block_volume.app_data.zone
}

output "block_volume_size_in_gb" {
  description = "Size of the application data block volume in GB"
  value       = scaleway_block_volume.app_data.size_in_gb
}

output "block_volume_iops" {
  description = "Provisioned IO/s of the application data block volume"
  value       = scaleway_block_volume.app_data.iops
}

output "region" {
  description = "Region the stack is deployed to"
  value       = var.region
}

output "zone" {
  description = "Zone the stack is deployed to"
  value       = var.zone
}