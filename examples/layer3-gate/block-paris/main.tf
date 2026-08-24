resource "scaleway_account_project" "main" {
  name        = var.project_name
  description = var.project_description
}