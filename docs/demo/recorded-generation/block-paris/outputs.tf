output "project_id" {
  description = "Bootstrapped project id owning all scenario resources"
  value       = scaleway_account_project.main.id
}

output "block_volume_id" {
  description = "Zoned id of the app-data block volume for destruction/orphan verification"
  value       = scaleway_block_volume.app_data.id
}

output "block_volume_zone" {
  description = "Zone the volume was provisioned in, for region_restriction policy checks"
  value       = scaleway_block_volume.app_data.zone
}