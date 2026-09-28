package scaleway.region_restriction_test

import rego.v1

import data.scaleway.region_restriction

paris := {"region": {"constant_value": "fr-par"}, "zone": {"constant_value": "fr-par-1"}}

in_fr_par := {"region": "fr-par"}

# Trimmed from `tofu show -json` of a scaleway/scaleway 2.83.0 plan
# against mockway. The provider leaves `zone` and `region` unknown in
# planned_values even when the resource block sets them, so they are
# absent there and appear only under `configuration`.
plan(address, expressions, provider, params) := {
	"planned_values": {"root_module": {"resources": [{
		"address": address,
		"mode": "managed",
		"type": type,
		"name": name,
		"provider_name": "registry.opentofu.org/scaleway/scaleway",
		"values": {"name": name},
	}]}},
	"configuration": {
		"provider_config": {"scaleway": {
			"name": "scaleway",
			"full_name": "registry.opentofu.org/scaleway/scaleway",
			"expressions": provider,
		}},
		"root_module": {"resources": [{
			"address": address,
			"mode": "managed",
			"type": type,
			"name": name,
			"provider_config_key": "scaleway",
			"expressions": expressions,
		}]},
	},
	"variables": {"zone": {"value": "nl-ams-1"}},
	"params": params,
} if {
	[type, name] := split(address, ".")
}

# Trimmed from mockway GET /mock/state; params is what infrafactory adds
# from the criterion.
server_state(zone, params) := {
	"instance": {"servers": [{
		"id": "6f1c1b0e-6b0b-4d8e-9f5a-0f3c2a1d9e11",
		"name": "web",
		"commercial_type": "DEV1-S",
		"state": "stopped",
		"zone": zone,
	}]},
	"params": params,
}

test_zone_outside_region_denies if {
	region_restriction.deny == {"scaleway_instance_server.web is in zone nl-ams-1 — must be in fr-par"} with input as plan(
		"scaleway_instance_server.web",
		{"zone": {"constant_value": "nl-ams-1"}},
		paris,
		in_fr_par,
	)
}

test_zone_from_variable_denies if {
	region_restriction.deny == {"scaleway_instance_server.web is in zone nl-ams-1 — must be in fr-par"} with input as plan(
		"scaleway_instance_server.web",
		{"zone": {"references": ["var.zone"]}},
		paris,
		in_fr_par,
	)
}

test_zone_inside_region_passes if {
	count(region_restriction.deny) == 0 with input as plan(
		"scaleway_instance_server.web",
		{"zone": {"constant_value": "fr-par-2"}},
		paris,
		in_fr_par,
	)
}

test_zone_param_is_exact if {
	region_restriction.deny == {"scaleway_instance_server.web is in zone fr-par-2 — must be in fr-par-1"} with input as plan(
		"scaleway_instance_server.web",
		{"zone": {"constant_value": "fr-par-2"}},
		paris,
		{"region": "fr-par", "zone": "fr-par-1"},
	)
}

test_region_outside_denies if {
	region_restriction.deny == {"scaleway_rdb_instance.main is in region nl-ams — must be in fr-par"} with input as plan(
		"scaleway_rdb_instance.main",
		{"region": {"constant_value": "nl-ams"}},
		paris,
		in_fr_par,
	)
}

test_provider_defaults_outside_region_deny if {
	region_restriction.deny == {
		"provider scaleway defaults to region nl-ams — must be in fr-par",
		"provider scaleway defaults to zone nl-ams-1 — must be in fr-par",
	} with input as plan(
		"scaleway_instance_server.web",
		{},
		{"region": {"constant_value": "nl-ams"}, "zone": {"references": ["var.zone"]}},
		in_fr_par,
	)
}

test_state_zone_outside_region_denies if {
	region_restriction.deny_state == {"web is in zone nl-ams-1 in the deployed state — must be in fr-par"} with input as server_state("nl-ams-1", in_fr_par)
}

test_state_zone_param_is_exact if {
	region_restriction.deny_state == {"web is in zone fr-par-2 in the deployed state — must be in fr-par-1"} with input as server_state("fr-par-2", {"region": "fr-par", "zone": "fr-par-1"})
}

test_state_region_outside_denies if {
	region_restriction.deny_state == {"pn is in region nl-ams in the deployed state — must be in fr-par"} with input as {
		"vpc": {"private_networks": [{
			"id": "0d9b3c5e-2f4a-4c1e-8b7d-3a6e5f9c1b22",
			"name": "pn",
			"region": "nl-ams",
			"vpc_id": "9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d",
		}]},
		"params": in_fr_par,
	}
}
