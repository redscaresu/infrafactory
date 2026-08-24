resource "scaleway_account_project" "main" {
  name = "if-s145-iam-scope"
}

# A managed PostgreSQL instance. The deploy allowlist permits it; the
# credential running the apply is deliberately not allowed to create one.
resource "scaleway_rdb_instance" "app" {
  name           = "if-s145-db"
  node_type      = "DB-DEV-S"
  engine         = "PostgreSQL-15"
  is_ha_cluster  = false
  disable_backup = true
  user_name      = "appuser"
  password       = "Refused-Before-Use-1!"
  project_id     = scaleway_account_project.main.id
}
