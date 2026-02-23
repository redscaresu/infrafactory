package common.naming

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	name := resource.values.name
	name != null
	not regex.match(`^[a-z](?:[a-z0-9-]*[a-z0-9])?$`, name)
	msg := sprintf(
		"%s has name '%s' - must start with a lowercase letter, use lowercase alphanumeric or hyphens, and not end with a hyphen",
		[resource.address, name],
	)
}
