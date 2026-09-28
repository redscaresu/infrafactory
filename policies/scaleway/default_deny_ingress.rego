package scaleway.default_deny_ingress

import rego.v1

# A security group that defaults to `accept` is not a firewall.
#
# VERIFIED, not inferred (2026-09-27, real Scaleway, provider 2.83.0):
# a scaleway_instance_security_group declaring only a name applies with
# inbound_default_policy = "accept" and outbound_default_policy =
# "accept" -- byte-identical in those fields to the API-created
# "Default security group". Declaring one without setting the policy
# closes nothing. The provider schema does not publish these defaults
# and the API spec says `default: unknown_policy`, so neither source
# answers the question; an apply does.
#
# Readable at Layer 1 because the provider renders its OWN default into
# the plan: an omitted attribute shows as the literal "accept" in
# planned_values with after_unknown false. That makes this the easy
# case, unlike vpc_required, whose subject is a cross-resource
# reference that stays unknown until apply and so needs a
# `configuration` fallback.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_instance_security_group"
	not inbound_is_drop(resource.values)
	# Denial text is repair-loop INPUT -- it is fed back to the
	# generator as the reason to fix, and since 2026-09-10 recorded
	# VERBATIM as a pitfall. Every claim here is therefore taught on
	# every iteration, so each one is checked above.
	msg := sprintf(
		"%s does not set `inbound_default_policy = \"drop\"`. A Scaleway security group defaults to accept inbound, so this group permits every port -- the same as the API-created default group, and not a firewall. Set it, and add an explicit `inbound_rule` for each port the scenario needs",
		[resource.address],
	)
}

# Same property against the Layer 2 mock's state after the mock apply.
# It reads the mock's record, never a cloud's, even on a run that also
# applied to a sandbox: the holdout is what checks the deployed stack
# for an open port (ADR-0034 section 4).
deny_state contains msg if {
	group := input.instance.security_groups[_]
	not is_api_default(group)
	not inbound_is_drop(group)
	msg := sprintf(
		"deployed security group %v (%v) has inbound default %v -- it permits every inbound port",
		[group.name, group.id, group.inbound_default_policy],
	)
}

# The API-created group is EXCLUDED, and that exclusion is the honest
# limit of this policy.
#
# Scaleway creates a "Default security group" in every project on the
# first instance, with inbound accept, and no generated HCL can change
# or remove it -- Terraform never owns it (which is why teardown has a
# purge step). Denying it would fail every run forever for a shape no
# iteration could repair, which is worse than not checking.
#
# So the property this policy defends is precisely "a security group
# THIS CONFIGURATION DECLARES is a real firewall". It is NOT "the
# server is behind a firewall": a configuration that declares no
# security group at all leaves the server on the API default and passes
# this policy. That gap is deliberate and named here rather than left
# to be discovered -- closing it needs a rule about the server, not the
# group, and the holdout in scenarios/holdout/ is what currently
# catches it.
is_api_default(group) if {
	group.project_default == true
}

is_api_default(group) if {
	group.organization_default == true
}

# Helper plus `not`, rather than `!= "drop"` inline, because an ABSENT
# field makes the comparison undefined and an undefined rule yields
# zero results -- indistinguishable from a rule that found nothing
# wrong. `not inbound_is_drop(...)` denies on absent, null, "accept"
# and anything else, so a future provider that stops rendering its
# default fails closed instead of silently passing. This is the same
# shape as vpc_required's `not has_private_nic(...)`, for the same
# reason.
inbound_is_drop(values) if {
	values.inbound_default_policy == "drop"
}
