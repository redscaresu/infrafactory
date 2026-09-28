package scaleway.vpc_required_test

import rego.v1

import data.scaleway.vpc_required

# Trimmed from `tofu show -json` of a scaleway/scaleway 2.83.0 plan. A new
# private network leaves the inline block as `[{}]` in planned_values, so
# the policy reads `configuration`, whose references are verbatim below.
server(address) := {
	"address": address,
	"mode": "managed",
	"type": "scaleway_instance_server",
	"provider_name": "registry.opentofu.org/scaleway/scaleway",
	"values": {"image": "ubuntu_noble", "name": "web", "type": "DEV1-S"},
}

config_server(address, refs) := {
	"address": address,
	"mode": "managed",
	"type": "scaleway_instance_server",
	"expressions": {"private_network": [{"pn_id": {"references": refs}}]},
}

config_nic(refs) := {
	"address": "scaleway_instance_private_nic.nic",
	"mode": "managed",
	"type": "scaleway_instance_private_nic",
	"expressions": {"server_id": {"references": refs}},
}

plan(planned, configured) := {
	"planned_values": {"root_module": {"resources": planned}},
	"configuration": {"root_module": {"resources": configured}},
}

deny_msg(address) := sprintf("%s is not attached to a private network. Add `private_network { pn_id = scaleway_vpc_private_network.NAME.id }` to the server. The Layer 3 gate refuses a standalone scaleway_instance_private_nic, so use the inline block", [address])

counted := [server("scaleway_instance_server.web[0]"), server("scaleway_instance_server.web[1]")]

test_bare_server_denies if {
	vpc_required.deny == {deny_msg("scaleway_instance_server.bare")} with input as plan([server("scaleway_instance_server.bare")], [])
}

test_singleton_nic_passes if {
	count(vpc_required.deny) == 0 with input as plan(
		[server("scaleway_instance_server.web")],
		[config_nic(["scaleway_instance_server.web.id", "scaleway_instance_server.web"])],
	)
}

test_counted_nic_passes if {
	count(vpc_required.deny) == 0 with input as plan(
		counted,
		[config_nic(["scaleway_instance_server.web", "count.index"])],
	)
}

test_nic_on_another_server_denies if {
	vpc_required.deny == {deny_msg("scaleway_instance_server.web")} with input as plan(
		[server("scaleway_instance_server.web")],
		[config_nic(["scaleway_instance_server.other.id", "scaleway_instance_server.other"])],
	)
}

test_inline_block_passes if {
	count(vpc_required.deny) == 0 with input as plan(
		[server("scaleway_instance_server.web")],
		[config_server("scaleway_instance_server.web", ["scaleway_vpc_private_network.main.id", "scaleway_vpc_private_network.main"])],
	)
}

# pn_id = scaleway_vpc_private_network.main[count.index].id
test_counted_inline_block_passes if {
	count(vpc_required.deny) == 0 with input as plan(
		counted,
		[config_server("scaleway_instance_server.web", ["scaleway_vpc_private_network.main", "count.index"])],
	)
}

# pn_id = scaleway_instance_server.inline.id: a block that is present and
# names no private network.
test_inline_block_wrong_reference_denies if {
	vpc_required.deny == {deny_msg("scaleway_instance_server.web")} with input as plan(
		[server("scaleway_instance_server.web")],
		[config_server("scaleway_instance_server.web", ["scaleway_instance_server.inline.id", "scaleway_instance_server.inline"])],
	)
}

# pn_id = scaleway_vpc_private_network.main.name: a real private network,
# not its id. Without count.index the bare reference does not pass.
test_inline_block_name_reference_denies if {
	vpc_required.deny == {deny_msg("scaleway_instance_server.web")} with input as plan(
		[server("scaleway_instance_server.web")],
		[config_server("scaleway_instance_server.web", ["scaleway_vpc_private_network.main.name", "scaleway_vpc_private_network.main"])],
	)
}
