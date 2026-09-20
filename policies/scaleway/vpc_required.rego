package scaleway.vpc_required

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "scaleway_instance_server"
	not has_private_nic(resource.address)
	# The denial text is repair-loop INPUT, not just a log line: it is fed
	# back to the generator as the reason to fix. Every claim in it is
	# therefore a claim this system teaches the model, and a false one is
	# taught on every iteration.
	#
	# It used to say the standalone NIC "cannot be destroyed". That was
	# true of provider 2.81.0 and is FALSE on 2.83.0: a standalone
	# scaleway_instance_private_nic applied and destroyed cleanly against
	# real Scaleway on 2026-09-20 (4 added, 4 destroyed, exit 0), because
	# upstream #4354 moved the teardown to a detach route that does not
	# refuse. The remaining reason is the true one and no more: the Layer 3
	# gate still refuses the shape.
	msg := sprintf(
		"%s is not attached to a private network. Add `private_network { pn_id = scaleway_vpc_private_network.NAME.id }` to the server. The Layer 3 gate refuses a standalone scaleway_instance_private_nic, so use the inline block",
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
# own inline block (schema: list, max 8) is another. This rule accepts
# BOTH, and always should have -- the invariant is the attachment.
#
# HISTORY, because the reason written here was wrong twice and the
# wrongness outlived both corrections:
#
#   - 2026-09-09: destroy failed with `Can't delete a private network
#     interface attached to a server`, and this comment concluded the
#     inline block was "the one that can be DESTROYED" because "the
#     provider powers the server off first".
#   - 2026-09-10: REFUTED (ADR-0029 Refutation). The inline block failed
#     identically. The provider does not power anything off; it detaches
#     the NIC as its own call either way.
#   - 2026-09-10: the real cause was the ENDPOINT (ADR-0031) -- provider
#     2.81.0 deleted NICs through v2alpha1, which refuses every NIC there
#     is.
#   - 2026-09-20: upstream #4354 fixed it by changing the endpoint, and
#     a standalone NIC now applies and destroys cleanly against real
#     Scaleway on 2.83.0 (4 added, 4 destroyed, exit 0).
#
# So neither shape is undestroyable any more, and the destroyability
# argument for preferring the inline block is gone. What remains is the
# Layer 3 gate, which still refuses the standalone resource -- now as a
# standardisation choice rather than a safety one. See ADR-0031.
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
	inline_pn_id_ref(server.expressions.private_network[_].pn_id.references)
}

# Both ends of the reference matter. tofu records BOTH
# "scaleway_vpc_private_network.main.id" and the bare
# "scaleway_vpc_private_network.main" for one expression, so a prefix-only
# check also accepts `pn_id = scaleway_vpc_private_network.main.name` -- a real
# private network, and not its id.
inline_pn_id_ref(refs) if {
	ref := refs[_]
	startswith(ref, "scaleway_vpc_private_network.")
	endswith(ref, ".id")
}

# ...except under `count`. For
# `pn_id = scaleway_vpc_private_network.main[count.index].id` tofu separates
# the resource from the dynamic index and records only the BARE reference plus
# "count.index" -- no ".id" anywhere. Demanding the suffix would refuse every
# multi-instance scenario, which is the count-shaped blind spot PR #8 fixed for
# the standalone NIC.
#
# `count.index` is what keeps a bare reference from admitting the SINGLETON
# `.name` form: bare alone is not enough, it has to be bare BECAUSE it is
# indexed.
#
# KNOWN LIMIT, stated rather than left to be discovered: within the counted
# case this rule cannot tell `.id` from `.name`. tofu records the identical
# references for both -- the bare resource plus "count.index" -- so the
# attribute is simply not in the plan JSON to test. The alternative, demanding
# a ".id" suffix, would deny every counted scenario at Layer 1 including the
# mock runs, which is a real cost against a hypothetical mistake.
#
# It is not the last line of defence. `layer3IsPrivateNetworkIDRef` reads the
# HCL itself, where the attribute IS present, and refuses anything not ending
# `.id` before a real apply. The imprecision lives only where it is cheap; the
# gate that guards money is exact.
#
# The standalone-NIC rules below have the same property and always have --
# `nic_refs_count_based` checks no attribute at all.
inline_pn_id_ref(refs) if {
	"count.index" in refs
	ref := refs[_]
	startswith(ref, "scaleway_vpc_private_network.")
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
