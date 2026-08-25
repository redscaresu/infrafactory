resource "scaleway_block_volume" "app_data" {
  name       = var.volume_name
  zone       = var.zone
  size_in_gb = var.volume_size_in_gb
  iops       = var.volume_iops
  project_id = scaleway_account_project.main.id
  tags       = var.volume_tags

  depends_on = [scaleway_account_project.main]
}