package scaleway.vpc_required

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_instance_server"
	not has_private_nic(resource.address)
	msg := sprintf(
		"%s is not attached to a private network via scaleway_instance_private_nic",
		[resource.address],
	)
}

has_private_nic(server_address) if {
	nic := input.planned_values.root_module.resources[_]
	nic.type == "scaleway_instance_private_nic"
	contains(nic.values.server_id, server_address)
}
