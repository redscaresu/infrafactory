package scaleway.default_deny_ingress_test

import rego.v1

import data.scaleway.default_deny_ingress

# Trimmed from `tofu show -json` of a scaleway/scaleway 2.83.0 plan. The
# "open" group declares only a name: the provider renders its own
# "accept" default into planned_values.
plan := {"planned_values": {"root_module": {"resources": [
	{
		"address": "scaleway_instance_security_group.open",
		"mode": "managed",
		"type": "scaleway_instance_security_group",
		"name": "open",
		"provider_name": "registry.opentofu.org/scaleway/scaleway",
		"values": {
			"enable_default_security": true,
			"inbound_default_policy": "accept",
			"name": "open-sg",
			"outbound_default_policy": "accept",
			"stateful": true,
		},
	},
	{
		"address": "scaleway_instance_security_group.closed",
		"mode": "managed",
		"type": "scaleway_instance_security_group",
		"name": "closed",
		"provider_name": "registry.opentofu.org/scaleway/scaleway",
		"values": {
			"enable_default_security": true,
			"inbound_default_policy": "drop",
			"name": "closed-sg",
			"outbound_default_policy": "accept",
			"stateful": true,
		},
	},
]}}}

# Trimmed from mockway GET /mock/state after applying the two groups above.
closed_group := {
	"enable_default_security": true,
	"id": "e9696623-be38-4c7b-9f82-f5dc450f69ff",
	"inbound_default_policy": "drop",
	"name": "closed-sg",
	"outbound_default_policy": "accept",
	"project": "00000000-0000-0000-0000-000000000000",
	"stateful": true,
	"zone": "fr-par-1",
}

open_group := {
	"enable_default_security": true,
	"id": "d64dfca0-6334-45f2-81b3-8f73fda0d3b7",
	"inbound_default_policy": "accept",
	"name": "open-sg",
	"outbound_default_policy": "accept",
	"project": "00000000-0000-0000-0000-000000000000",
	"stateful": true,
	"zone": "fr-par-1",
}

# The API-created group: mockway records only what the provider sends, so
# project_default / organization_default are the Scaleway Instance API's
# SecurityGroup fields, set here on the open group's shape.
api_default(field) := object.union(open_group, {
	"id": "5a1f3c0e-0000-4000-8000-000000000001",
	"name": "Default security group",
	field: true,
})

state(groups) := {"instance": {"security_groups": groups}}

test_plan_group_left_at_accept_denies if {
	default_deny_ingress.deny == {"scaleway_instance_security_group.open does not set `inbound_default_policy = \"drop\"`. A Scaleway security group defaults to accept inbound, so this group permits every port -- the same as the API-created default group, and not a firewall. Set it, and add an explicit `inbound_rule` for each port the scenario needs"} with input as plan
}

test_plan_group_without_policy_denies if {
	no_policy := {"planned_values": {"root_module": {"resources": [{
		"address": "scaleway_instance_security_group.open",
		"type": "scaleway_instance_security_group",
		"values": {"name": "open-sg"},
	}]}}}
	count(default_deny_ingress.deny) == 1 with input as no_policy
}

test_state_group_left_at_accept_denies if {
	default_deny_ingress.deny_state == {"deployed security group open-sg (d64dfca0-6334-45f2-81b3-8f73fda0d3b7) has inbound default accept -- it permits every inbound port"} with input as state([closed_group, open_group])
}

test_state_drop_group_passes if {
	count(default_deny_ingress.deny_state) == 0 with input as state([closed_group])
}

test_state_project_default_group_passes if {
	count(default_deny_ingress.deny_state) == 0 with input as state([closed_group, api_default("project_default")])
}

test_state_organization_default_group_passes if {
	count(default_deny_ingress.deny_state) == 0 with input as state([closed_group, api_default("organization_default")])
}
