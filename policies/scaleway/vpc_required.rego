package scaleway.vpc_required

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_instance_server"
	not has_private_nic(resource.address)
	msg := sprintf(
		"%s is not attached to a private network via scaleway_instance_private_nic",
		[resource.address],
	)
}

# has_private_nic looks for any scaleway_instance_private_nic whose
# `server_id` expression references the given server.
#
# Reference shapes vary by HCL pattern:
#   - Singleton server + singleton NIC: refs = "scaleway_instance_server.web.id".
#     planned_values address: "scaleway_instance_server.web". Comparing
#     bare-address + ".id" works directly.
#   - count-based server + count-based NIC ("scaleway_instance_private_nic" "web"
#     with count, server_id = scaleway_instance_server.web[count.index].id):
#     tofu's configuration.root_module.resources[].expressions.server_id.references
#     stores the SYMBOLIC reference, which is "scaleway_instance_server.web"
#     (no [N] — the count.index is dynamic). planned_values addresses include
#     the concrete index ("scaleway_instance_server.web[0]",
#     "scaleway_instance_server.web[1]", ...).
#
# The fix: strip any trailing [N] index from the planned address before
# comparing to the symbolic reference. This is correct because if a NIC
# whose server_id references the parent collection exists, it's
# implicitly attaching all instances of that count-based server (tofu's
# `count` semantics fan it out 1:1).
#
# Surfaced in the 2026-06-01 deterministic sweep: web-app-paris +
# compute-lb-multi-paris use count-based servers + matching count-based
# NICs (exactly as the prescriptive `scaleway_instance_server` pitfall
# recommends) and were nonetheless flagged by this policy, sending the
# LLM into an unbreakable iter1↔iter2 oscillation. Pre-fix the policy
# disagreed with its own pitfall.
has_private_nic(server_address) if {
	nic := input.configuration.root_module.resources[_]
	nic.type == "scaleway_instance_private_nic"
	refs := nic.expressions.server_id.references
	bare_address := regex.replace(server_address, `\[\d+\]$`, "")
	refs[_] == sprintf("%s.id", [bare_address])
}
