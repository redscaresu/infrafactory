package scaleway.no_public_database

import rego.v1

# Layer 1: check against tofu plan JSON
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_rdb_instance"
	not resource.values.private_network
	msg := sprintf(
		"%s has no private_network — public access allowed",
		[resource.address],
	)
}

# Layer 2: check against ScalewayMock state
deny_state contains msg if {
	instance := input.state.rdb.instances[_]
	endpoint := instance.endpoints[_]
	not endpoint.private_network
	msg := sprintf(
		"RDB %s has public endpoint in deployed state",
		[instance.id],
	)
}
