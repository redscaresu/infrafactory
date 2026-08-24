resource "scaleway_account_project" "main" {
  name = "if-s145-iam-scope"
}

# A DNS zone. scaleway_domain* is inside the deploy allowlist; the
# credential running the apply is deliberately not permitted to create
# one, and never will be.
resource "scaleway_domain_zone" "app" {
  domain     = "infrafactory-demo.com"
  subdomain  = "gate"
  project_id = scaleway_account_project.main.id
}
