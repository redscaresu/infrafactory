package scaleway.no_public_endpoints

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_instance_server"
	ip := resource.values.public_ip
	ip != null
	ip != ""
	msg := sprintf(
		"%s has a public IP assigned — should use private networking only",
		[resource.address],
	)
}

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_instance_ip"
	server := resource.values.server_id
	server != null
	msg := sprintf(
		"%s assigns a public IP to a server — violates no_public_endpoints",
		[resource.address],
	)
}
