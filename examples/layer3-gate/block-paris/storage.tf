resource "scaleway_block_volume" "app_data" {
  name       = var.block_volume_name
  zone       = var.zone
  iops       = var.block_volume_iops
  size_in_gb = var.block_volume_size_in_gb
  project_id = scaleway_account_project.main.id
  tags       = var.tags

  depends_on = [scaleway_account_project.main]
}