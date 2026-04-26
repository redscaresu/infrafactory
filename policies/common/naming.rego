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
	not is_gcp_resource_path(name)
	not is_dns_fqdn(resource, name)
	not regex.match(`^[a-z](?:[a-z0-9-]*[a-z0-9])?$`, name)
	msg := sprintf(
		"%s has name '%s' - must start with a lowercase letter, use lowercase alphanumeric or hyphens, and not end with a hyphen",
		[resource.address, name],
	)
}

is_gcp_resource_path(name) if {
	startswith(name, "projects/")
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
