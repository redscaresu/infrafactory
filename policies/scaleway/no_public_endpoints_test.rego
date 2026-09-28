package scaleway.no_public_endpoints_test

import rego.v1

import data.scaleway.no_public_endpoints

# Trimmed from `tofu show -json` of scaleway/scaleway 2.83.0 plans
# against mockway, for a server with ip_id = scaleway_instance_ip.web.id.
resource(type, name, values) := {
	"address": sprintf("%s.%s", [type, name]),
	"mode": "managed",
	"type": type,
	"name": name,
	"provider_name": "registry.opentofu.org/scaleway/scaleway",
	"values": values,
}

server(values) := resource("scaleway_instance_server", "web", object.union({"image": "ubuntu_jammy", "name": "web", "type": "DEV1-S"}, values))

plan(resources, configuration) := {
	"planned_values": {"root_module": {"resources": resources}},
	"configuration": {"root_module": {"resources": configuration}},
}

ip_config := {
	"address": "scaleway_instance_ip.web",
	"mode": "managed",
	"type": "scaleway_instance_ip",
	"name": "web",
	"provider_config_key": "scaleway",
}

server_config(expressions) := {
	"address": "scaleway_instance_server.web",
	"mode": "managed",
	"type": "scaleway_instance_server",
	"name": "web",
	"provider_config_key": "scaleway",
	"expressions": expressions,
}

# A re-plan: the IP comes from prior state with every attribute known.
replanned_ip(server_id) := resource("scaleway_instance_ip", "web", {
	"address": "51.15.98.51",
	"id": "fr-par-1/30f319d8-1da3-4468-854a-9bbe01bb85b2",
	"project_id": "00000000-0000-0000-0000-000000000000",
	"server_id": server_id,
	"zone": "fr-par-1",
})

bound_denial := {"scaleway_instance_ip.web assigns a public IP to a server — violates no_public_endpoints"}

test_replanned_bound_ip_denies if {
	no_public_endpoints.deny == bound_denial with input as plan(
		[replanned_ip("fr-par-1/c3fc4298-8573-4aea-b7ef-31ed663a5a2e"), server({"ip_id": "fr-par-1/30f319d8-1da3-4468-854a-9bbe01bb85b2"})],
		[],
	)
}

# Pinned limit: on a fresh plan server_id is unknown, so the IP's values
# are empty and the binding appears only in configuration.
test_fresh_ip_id_does_not_deny if {
	count(no_public_endpoints.deny) == 0 with input as plan(
		[resource("scaleway_instance_ip", "web", {}), server({"enable_dynamic_ip": false, "ip_ids": null})],
		[ip_config, server_config({"ip_id": {"references": ["scaleway_instance_ip.web.id", "scaleway_instance_ip.web"]}})],
	)
}

# Pinned limit: ip_ids is not checked.
test_ip_ids_does_not_deny if {
	count(no_public_endpoints.deny) == 0 with input as plan(
		[resource("scaleway_instance_ip", "web", {}), server({"enable_dynamic_ip": false})],
		[ip_config, server_config({"ip_ids": {"references": ["scaleway_instance_ip.web.id", "scaleway_instance_ip.web"]}})],
	)
}

# Pinned limit: enable_dynamic_ip is not checked.
test_enable_dynamic_ip_does_not_deny if {
	count(no_public_endpoints.deny) == 0 with input as plan(
		[server({"enable_dynamic_ip": true, "ip_ids": null})],
		[server_config({"enable_dynamic_ip": {"constant_value": true}})],
	)
}
