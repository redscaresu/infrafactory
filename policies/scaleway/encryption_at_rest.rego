package scaleway.encryption_at_rest

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_rdb_instance"
	not resource.values.encryption_at_rest
	msg := sprintf(
		"%s does not have encryption_at_rest enabled",
		[resource.address],
	)
}

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_object_bucket"
	not resource.values.versioning
	msg := sprintf(
		"%s does not have versioning enabled (required for encryption compliance)",
		[resource.address],
	)
}
