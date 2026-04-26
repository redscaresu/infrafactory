package common.naming

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	name := resource.values.name
	name != null
	# Skip names that are clearly not user-controlled slug-style identifiers:
	#   - fully-qualified GCP resource paths (`projects/p/secrets/s/...`)
	#   - DNS record names / FQDNs (`host.example.invalid.`)
	# Both follow protocol-defined formats, not the lowercase-alphanumeric
	# slug convention this rule was written for.
	not contains(name, "/")
	not contains(name, ".")
	not regex.match(`^[a-z](?:[a-z0-9-]*[a-z0-9])?$`, name)
	msg := sprintf(
		"%s has name '%s' - must start with a lowercase letter, use lowercase alphanumeric or hyphens, and not end with a hyphen",
		[resource.address, name],
	)
}
