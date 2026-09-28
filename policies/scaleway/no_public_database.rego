package scaleway.no_public_database

import rego.v1

# Layer 1: check against tofu plan JSON. An absent private_network block
# plans as [], which `not` lets through, so require an element.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_rdb_instance"
	not resource.values.private_network[0]
	msg := sprintf(
		"%s has no private_network — public access allowed",
		[resource.address],
	)
}

# Layer 2: check against the Layer 2 mock's state (mockway), never a
# cloud's (ADR-0034 section 4). A public endpoint's private_network is
# absent or null, so anything but an object is public. object.get, because
# OPA evaluates a builtin's argument outside the `not`: without it an absent
# key makes the expression undefined rather than true.
deny_state contains msg if {
	instance := input.rdb.instances[_]
	endpoint := instance.endpoints[_]
	not is_object(object.get(endpoint, "private_network", null))
	msg := sprintf(
		"RDB %s has public endpoint in deployed state",
		[instance.id],
	)
}
