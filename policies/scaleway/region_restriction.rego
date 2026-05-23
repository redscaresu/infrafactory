package scaleway.region_restriction

import rego.v1

deny contains msg if {
	allowed := input.params.region
	resource := input.planned_values.root_module.resources[_]
	region := resource.values.region
	region != null
	not startswith(region, allowed)
	msg := sprintf(
		"%s is in region %s — must be in %s",
		[resource.address, region, allowed],
	)
}

deny contains msg if {
	allowed := input.params.zone
	allowed != null
	resource := input.planned_values.root_module.resources[_]
	zone := resource.values.zone
	zone != null
	zone != allowed
	msg := sprintf(
		"%s is in zone %s — must be in %s",
		[resource.address, zone, allowed],
	)
}
