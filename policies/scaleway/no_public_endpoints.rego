package scaleway.no_public_endpoints

import rego.v1

# Denies a scaleway_instance_ip whose planned server_id is known. That
# happens only on a re-plan, when the IP comes from prior state: a fresh
# plan leaves server_id unknown, so it is absent from planned_values. A
# known empty server_id, an unbound IP from prior state, denies too.
#
# Not checked: a server's ip_id (known only in configuration on a fresh
# plan), its ip_ids, or enable_dynamic_ip.
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
