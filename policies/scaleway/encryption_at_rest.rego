package scaleway.encryption_at_rest

import rego.v1

# Only an explicit true passes. An omitted encryption_at_rest plans as null,
# which `not` lets through; object.get covers an absent key, where
# `x.k != true` would be undefined rather than true.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_rdb_instance"
	object.get(resource.values, "encryption_at_rest", null) != true
	msg := sprintf(
		"%s does not have encryption_at_rest enabled",
		[resource.address],
	)
}
