package common.naming

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	name := resource.values.name
	name != null
	# Skip names that are clearly server-assigned or protocol-defined
	# values rather than user-given slug identifiers:
	#   - GCP fully-qualified resource paths begin with `projects/`
	#     (e.g. `projects/p/secrets/s` for google_secret_manager_secret).
	#   - DNS record FQDNs end with a trailing dot, but only on
	#     resources we know expect that shape (google_dns_*). Without
	#     the resource-type guard a typo'd compute / storage name like
	#     "api." or "bucket." would slip through.
	not is_gcp_resource_path(resource, name)
	not is_dns_fqdn(resource, name)
	not regex.match(`^[a-z](?:[a-z0-9-]*[a-z0-9])?$`, name)
	msg := sprintf(
		"%s has name '%s' - must start with a lowercase letter, use lowercase alphanumeric or hyphens, and not end with a hyphen",
		[resource.address, name],
	)
}

is_gcp_resource_path(resource, name) if {
	# Only resource types whose `name` attribute is server-assigned
	# to a fully-qualified GCP resource path skip the slug rule. For
	# everything else — google_pubsub_topic, google_storage_bucket,
	# google_dns_managed_zone, etc. — `name` is a user-controlled
	# slug and a leading "projects/" is an actual misconfiguration.
	gcp_path_named_types[resource.type]
	startswith(name, "projects/")
}

# gcp_path_named_types is the set of google_* resources whose `name`
# in tofu state is the server-assigned fully-qualified path. The
# Terraform google provider exposes the user-controlled leaf via a
# different attribute (secret_id, account_id, etc.) — `name` is for
# reading back, not for setting.
gcp_path_named_types := {
	"google_secret_manager_secret",
	"google_secret_manager_secret_version",
	"google_service_account",
	"google_service_account_key",
}

is_dns_fqdn(resource, name) if {
	# Only google_dns_record_set actually models its `name` as a DNS
	# FQDN (e.g. "host.example.invalid."). google_dns_managed_zone.name
	# is a regular slug like "test-zone" and must still pass the
	# lowercase-alphanumeric check, so we don't exempt the whole
	# `google_dns_*` family.
	resource.type == "google_dns_record_set"
	endswith(name, ".")
}
