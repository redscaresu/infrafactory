resource "scaleway_block_volume" "app_data" {
  name       = var.block_volume_name
  zone       = var.zone
  iops       = var.block_volume_iops
  size_in_gb = var.block_volume_size_in_gb
  tags       = var.tags
}