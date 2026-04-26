package common.naming

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	name := resource.values.name
	name != null
	# Skip names that are clearly server-assigned, protocol-defined values
	# rather than user-given slug identifiers:
	#   - GCP fully-qualified resource paths begin with `projects/`
	#     (e.g. `projects/p/secrets/s` for google_secret_manager_secret).
	#   - DNS record FQDNs end with a trailing dot (`host.example.invalid.`).
	# Anything else — including names that merely contain a single `.` or
	# `/` somewhere in the middle — is still validated, so genuine
	# misconfigurations like "My.Bucket" or "foo/bar" still fail.
	not startswith(name, "projects/")
	not endswith(name, ".")
	not regex.match(`^[a-z](?:[a-z0-9-]*[a-z0-9])?$`, name)
	msg := sprintf(
		"%s has name '%s' - must start with a lowercase letter, use lowercase alphanumeric or hyphens, and not end with a hyphen",
		[resource.address, name],
	)
}
