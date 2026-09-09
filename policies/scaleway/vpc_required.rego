package scaleway.vpc_required

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_instance_server"
	not has_private_nic(resource.address)
	# The denial text is repair-loop INPUT, not just a log line: it is fed
	# back to the generator as the reason to fix. Naming the standalone NIC
	# here would send the next iteration straight back to the shape this
	# policy was just changed to stop mandating -- the fix undone by its own
	# error message.
	msg := sprintf(
		"%s is not attached to a private network. Add a `private_network { pn_id = ... }` block to the server (preferred -- it is destroyed with the server), or a scaleway_instance_private_nic referencing it",
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

# Third shape: the attachment declared INLINE on the server itself.
#
#   resource "scaleway_instance_server" "web" {
#     private_network { pn_id = scaleway_vpc_private_network.main.id }
#   }
#
# The property this policy defends is "the server is on a private
# network", not "a particular resource type appears". A standalone
# `scaleway_instance_private_nic` is one way to say it; the provider's
# own inline block (schema 2.81.0: list, max 8) is another, and it is the
# one that can be DESTROYED.
#
# 2026-09-09, real Scaleway, web-live-paris: apply succeeded and destroy
# failed with `Can't delete a private network interface attached to a
# server`. Terraform destroys in reverse dependency order, so a
# standalone NIC is deleted while its server is still RUNNING, and
# Scaleway refuses that. Deleting the server takes its NICs with it --
# the provider powers the server off first -- so the inline block has no
# separate delete to fail. Requiring only the standalone shape meant
# Layer 1 mandating a stack that could not be torn down, which is the
# S168 defect class exactly.
#
# Checked against `configuration`, not `planned_values`: for a new
# private network the block's values are all unknown at plan time and
# planned_values renders it as `[{}]` -- present but empty, which proves
# nothing. The expression's `references` are what tie it to a real
# private network.
has_private_nic(server_address) if {
	bare_address := regex.replace(server_address, `\[\d+\]$`, "")
	server := input.configuration.root_module.resources[_]
	server.type == "scaleway_instance_server"
	server.address == bare_address
	# The reference must name a private NETWORK. Accepting any reference
	# would pass `pn_id = scaleway_instance_server.web.id` -- a block that
	# is present, wrong, and only discovered at apply, which is the whole
	# thing Layer 1 exists to prevent.
	startswith(server.expressions.private_network[_].pn_id.references[_], "scaleway_vpc_private_network.")
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
