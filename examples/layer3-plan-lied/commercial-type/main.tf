resource "scaleway_account_project" "main" {
  name = "if-s145-a"
}
resource "scaleway_block_volume" "app_data" {
  name       = "s145-a-vol"
  iops       = 9000
  size_in_gb = 1
  project_id = scaleway_account_project.main.id
}
