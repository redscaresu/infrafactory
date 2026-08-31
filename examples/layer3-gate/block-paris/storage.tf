resource "scaleway_block_volume" "app_data" {
  name       = var.volume_name
  zone       = var.zone
  size_in_gb = var.volume_size_in_gb
  iops       = var.volume_iops
  tags       = var.volume_tags
}