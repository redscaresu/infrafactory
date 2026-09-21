package scaleway.region_restriction

import rego.v1

deny contains msg if {
	allowed := input.params.region
	resource := input.planned_values.root_module.resources[_]
	region := resource.values.region
	region != null
	not startswith(region, allowed)
	msg := sprintf(
		"%s is in region %s — must be in %s",
		[resource.address, region, allowed],
	)
}

deny contains msg if {
	allowed := input.params.zone
	allowed != null
	resource := input.planned_values.root_module.resources[_]
	zone := resource.values.zone
	zone != null
	zone != allowed
	msg := sprintf(
		"%s is in zone %s — must be in %s",
		[resource.address, zone, allowed],
	)
}

# Layer 2: the same question, asked of what the provider actually
# created.
#
# The plan rules above read the CONFIG's declared region. These read
# the deployed state, which is a different claim: a provider can accept
# `region = fr-par` and create somewhere else, and only the second
# reading would notice.
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
