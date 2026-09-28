package scaleway.region_restriction

import rego.v1

deny contains msg if {
	allowed := input.params.region
	resource := input.planned_values.root_module.resources[_]
	region := placement(resource, "region")
	not startswith(region, allowed)
	msg := sprintf(
		"%s is in region %s — must be in %s",
		[resource.address, region, allowed],
	)
}

# A zone satisfies a region: `fr-par-1` is in `fr-par`. Zonal resources
# such as scaleway_instance_server carry a zone and no region, so the
# region rule above never sees them.
deny contains msg if {
	allowed := input.params.region
	resource := input.planned_values.root_module.resources[_]
	zone := placement(resource, "zone")
	not startswith(zone, allowed)
	msg := sprintf(
		"%s is in zone %s — must be in %s",
		[resource.address, zone, allowed],
	)
}

deny contains msg if {
	allowed := input.params.zone
	allowed != null
	resource := input.planned_values.root_module.resources[_]
	zone := placement(resource, "zone")
	zone != allowed
	msg := sprintf(
		"%s is in zone %s — must be in %s",
		[resource.address, zone, allowed],
	)
}

# A resource that sets no zone or region of its own takes the provider's,
# and generated HCL usually sets the zone only there.
deny contains msg if {
	allowed := input.params.region
	some key, provider in input.configuration.provider_config
	provider.name == "scaleway"
	some attr in ["region", "zone"]
	value := expression_value(provider.expressions[attr])
	not startswith(value, allowed)
	msg := sprintf(
		"provider %s defaults to %s %s — must be in %s",
		[key, attr, value, allowed],
	)
}

# placement is the zone or region a planned resource is declared in.
# Provider 2.83.0 leaves both unknown in planned_values even when the
# resource block sets them, so the plan's `configuration` is read when
# the planned value is not known.
placement(resource, attr) := value if {
	value := resource.values[attr]
	is_string(value)
} else := value if {
	block := input.configuration.root_module.resources[_]
	block.mode == resource.mode
	block.type == resource.type
	block.name == resource.name
	value := expression_value(block.expressions[attr])
}

# ponytail: a literal or a lone `var.NAME` reference. A local's value is
# not in the plan JSON, so a zone set from one is left to deny_state at
# Layer 2; an interpolation of one variable reads as that variable.
expression_value(expr) := expr.constant_value if is_string(expr.constant_value)

expression_value(expr) := value if {
	[ref] := expr.references
	name := trim_prefix(ref, "var.")
	name != ref
	value := input.variables[name].value
	is_string(value)
}

# Layer 2: the same question, asked of the Layer 2 mock's state.
#
# The plan rules above read the CONFIG's declared region. These read
# the region and zone the mock recorded at apply -- the mock's record,
# never a cloud's, even on a run that also applied to a sandbox (ADR-0034
# section 4).
#
# This rule could not be written until the state evaluator passed the
# criterion's `params` through. Before that `input.params.region` was
# undefined here, so the rule would never have fired and would have
# reported a clean pass -- a check that cannot run is worse than no
# check, because it reads as coverage.

# state_resource is every object in every collection of the mock's
# state, whatever cloud or service it belongs to.
#
# Walked generically rather than listing `input.instance.servers`,
# `input.lb.lbs` and so on: an explicit list silently stops covering
# each new resource type, and a region rule that quietly skips the
# resource you just added is the failure mode this whole policy exists
# to prevent. The envelope keys are skipped because they are inputs,
# not resources.
state_resource contains resource if {
	some group, collection
	not group in {"params", "state", "target"}
	items := input[group][collection]
	is_array(items)
	resource := items[_]
	is_object(resource)
}

# A zone satisfies a region: `fr-par-1` is in `fr-par`. Same
# startswith comparison the plan rule uses, for the same reason.
deny_state contains msg if {
	allowed := input.params.region
	resource := state_resource[_]
	region := resource.region
	region != null
	not startswith(region, allowed)
	msg := sprintf(
		"%s is in region %s in the deployed state — must be in %s",
		[state_resource_label(resource), region, allowed],
	)
}

deny_state contains msg if {
	allowed := input.params.region
	resource := state_resource[_]
	zone := resource.zone
	zone != null
	not startswith(zone, allowed)
	msg := sprintf(
		"%s is in zone %s in the deployed state — must be in %s",
		[state_resource_label(resource), zone, allowed],
	)
}

# A stricter `zone` param is an EXACT match, mirroring the plan rule
# above. Without this a criterion asking for `zone: fr-par-1` is
# satisfied by a resource in fr-par-2, because the region rule only
# checks the `fr-par` prefix -- so the deployed-state check would be
# weaker than the plan check it is supposed to confirm.
deny_state contains msg if {
	allowed := input.params.zone
	allowed != null
	resource := state_resource[_]
	zone := resource.zone
	zone != null
	zone != allowed
	msg := sprintf(
		"%s is in zone %s in the deployed state — must be in %s",
		[state_resource_label(resource), zone, allowed],
	)
}

# Named by whatever the resource carries. An id is worse than a name
# and better than nothing: the operator has to find the thing.
state_resource_label(resource) := object.get(
	resource,
	"name",
	object.get(resource, "id", "resource"),
)
