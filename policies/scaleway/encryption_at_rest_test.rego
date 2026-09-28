package scaleway.encryption_at_rest_test

import rego.v1

import data.scaleway.encryption_at_rest

# Trimmed from `tofu show -json` of a scaleway/scaleway 2.83.0 plan.
rdb_plan(values) := {"planned_values": {"root_module": {"resources": [{
	"address": "scaleway_rdb_instance.db",
	"mode": "managed",
	"type": "scaleway_rdb_instance",
	"name": "db",
	"provider_name": "registry.opentofu.org/scaleway/scaleway",
	"values": object.union({"engine": "PostgreSQL-15", "name": "db", "node_type": "DB-DEV-S"}, values),
}]}}}

unencrypted := {"scaleway_rdb_instance.db does not have encryption_at_rest enabled"}

# What 2.83.0 plans for an omitted encryption_at_rest.
test_omitted_encryption_denies if {
	encryption_at_rest.deny == unencrypted with input as rdb_plan({"encryption_at_rest": null})
}

test_absent_encryption_denies if {
	encryption_at_rest.deny == unencrypted with input as rdb_plan({})
}

test_encryption_enabled_passes if {
	count(encryption_at_rest.deny) == 0 with input as rdb_plan({"encryption_at_rest": true})
}
