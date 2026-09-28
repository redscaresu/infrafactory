package scaleway.no_public_database_test

import rego.v1

import data.scaleway.no_public_database

# Trimmed from `tofu show -json` of a scaleway/scaleway 2.83.0 plan. An
# absent private_network block plans as []; a present one as a list of one
# object whose pn_id is still unknown.
rdb_plan(private_network) := {"planned_values": {"root_module": {"resources": [{
	"address": "scaleway_rdb_instance.db",
	"mode": "managed",
	"type": "scaleway_rdb_instance",
	"name": "db",
	"provider_name": "registry.opentofu.org/scaleway/scaleway",
	"values": {
		"engine": "PostgreSQL-15",
		"name": "db",
		"node_type": "DB-DEV-S",
		"private_network": private_network,
	},
}]}}}

# Trimmed from mockway GET /mock/state after applying that plan.
rdb_state(endpoint) := {"rdb": {"instances": [{
	"id": "903f527c-ea53-4039-9acd-463874349215",
	"name": "db",
	"engine": "PostgreSQL-15",
	"region": "fr-par",
	"endpoints": [object.union({"id": "22745ad2-d031-4d93-a21f-d4e9acb0d225", "port": 5432}, endpoint)],
}]}}

public_denial := {"RDB 903f527c-ea53-4039-9acd-463874349215 has public endpoint in deployed state"}

test_no_private_network_block_denies if {
	no_public_database.deny == {"scaleway_rdb_instance.db has no private_network — public access allowed"} with input as rdb_plan([])
}

test_private_network_block_passes if {
	count(no_public_database.deny) == 0 with input as rdb_plan([{"enable_ipam": true}])
}

# What mockway renders for the provider's public init_endpoints entry.
test_public_endpoint_without_private_network_denies if {
	no_public_database.deny_state == public_denial with input as rdb_state({"ip": "51.15.51.103", "load_balancer": {}})
}

# What mockway's CreateRDBInstance defaults to when no init_endpoints is sent.
test_public_endpoint_with_null_private_network_denies if {
	no_public_database.deny_state == public_denial with input as rdb_state({"ip": "51.15.51.103", "load_balancer": {}, "private_network": null})
}

test_private_endpoint_passes if {
	count(no_public_database.deny_state) == 0 with input as rdb_state({"ip": "10.98.53.100", "private_network": {"id": "1e2fe791-9c7f-461e-abaa-8310b58fe40c"}})
}
