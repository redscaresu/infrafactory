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
#   - Singleton (server_id = scaleway_instance_server.web.id):
#       references = ["scaleway_instance_server.web.id"]
#   - Count-based (server_id = scaleway_instance_server.web[count.index].id):
#       references = ["scaleway_instance_server.web", "count.index"]
#     Note the BARE resource reference (no .id) — tofu separates the
#     resource ref from the dynamic [count.index].id attribute access.
#
# planned_values addresses include the concrete index for the count
# case ("scaleway_instance_server.web[0]", ...). We strip the trailing
# [N] and accept EITHER reference shape.
#
# The 2026-06-01 deterministic sweep first surfaced the count-based
# bug — pre-PR-#8 the policy ignored count entirely. PR #8 added the
# [N] strip but only matched the singleton `.id` ref. The 2026-06-02
# sweep showed compute-lb-multi-paris + web-app-paris + 2 others
# still failing because count-based NICs produce the bare ref. This
# revision accepts both shapes.
has_private_nic(server_address) if {
	bare_address := regex.replace(server_address, `\[\d+\]$`, "")
	nic_refs_singleton(bare_address)
}

has_private_nic(server_address) if {
	bare_address := regex.replace(server_address, `\[\d+\]$`, "")
	nic_refs_count_based(bare_address)
}

nic_refs_singleton(bare_address) if {
	nic := input.configuration.root_module.resources[_]
	nic.type == "scaleway_instance_private_nic"
	nic.expressions.server_id.references[_] == sprintf("%s.id", [bare_address])
}

nic_refs_count_based(bare_address) if {
	nic := input.configuration.root_module.resources[_]
	nic.type == "scaleway_instance_private_nic"
	nic.expressions.server_id.references[_] == bare_address
}
