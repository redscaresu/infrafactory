resource "scaleway_account_project" "main" {
  name = "if-s145-c"
}
resource "scaleway_registry_namespace" "images" {
  name       = "if-s145-c-images"
  is_public  = false
  project_id = scaleway_account_project.main.id
}
