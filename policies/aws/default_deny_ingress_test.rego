package aws.default_deny_ingress_test

import rego.v1

import data.aws.default_deny_ingress

# Trimmed from internal/harness/testdata/ingress/aws/, which capture.sh
# writes from hashicorp/aws 5.100.0 plans and fakeaws /mock/state.
# internal/harness/aws_default_deny_ingress_test.go runs the whole
# captures.

change(address, type, after, after_unknown) := {
	"address": address,
	"mode": "managed",
	"type": type,
	"name": regex.replace(split(address, ".")[1], `\[.*`, ""),
	"change": {"actions": ["create"], "after": after, "after_unknown": after_unknown},
}

plan(changes) := {"resource_changes": changes}

ssh_ipv4 := change(
	"aws_security_group.open[\"ssh_ipv4\"]", "aws_security_group",
	{"ingress": [{"cidr_blocks": ["0.0.0.0/0"], "from_port": 22, "ipv6_cidr_blocks": [], "prefix_list_ids": [], "protocol": "tcp", "security_groups": [], "self": false, "to_port": 22}]},
	{"ingress": [{"cidr_blocks": [false], "ipv6_cidr_blocks": [], "prefix_list_ids": [], "security_groups": []}]},
)

test_inline_public_ssh_denies if {
	default_deny_ingress.deny == {"aws_security_group.open[\"ssh_ipv4\"] admits tcp 22-22, which includes SSH (22), from the public source 0.0.0.0/0. Open only the ports the scenario needs to the internet, such as tcp 80, and admit SSH or all traffic only from a literal private CIDR such as 10.0.0.0/16, or from a security group"} with input as plan([ssh_ipv4])
}

test_default_group_ipv6_ssh_denies if {
	count(default_deny_ingress.deny) == 1 with input as plan([change(
		"aws_default_security_group.open[\"ssh_ipv6\"]", "aws_default_security_group",
		{"ingress": [{"cidr_blocks": [], "from_port": 22, "ipv6_cidr_blocks": ["::/0"], "prefix_list_ids": [], "protocol": "tcp", "security_groups": [], "self": false, "to_port": 22}]},
		{"ingress": [{"cidr_blocks": [], "ipv6_cidr_blocks": [false], "prefix_list_ids": [], "security_groups": []}]},
	)])
}

# The provider rewrites "all" to "-1" and "6" to "tcp" on the other
# shapes, but keeps them as written here.
test_vpc_rule_all_word_denies if {
	some msg in default_deny_ingress.deny with input as plan([change(
		"aws_vpc_security_group_ingress_rule.open[\"all_word_0\"]", "aws_vpc_security_group_ingress_rule",
		{"cidr_ipv4": "0.0.0.0/0", "cidr_ipv6": null, "from_port": null, "ip_protocol": "all", "prefix_list_id": null, "to_port": null},
		{"arn": true, "id": true},
	)])
	contains(msg, "admits all traffic (protocol all) from the public source 0.0.0.0/0")
}

test_vpc_rule_six_denies if {
	some msg in default_deny_ingress.deny with input as plan([change(
		"aws_vpc_security_group_ingress_rule.open[\"ssh_six_0\"]", "aws_vpc_security_group_ingress_rule",
		{"cidr_ipv4": "0.0.0.0/0", "cidr_ipv6": null, "from_port": 22, "ip_protocol": "6", "prefix_list_id": null, "to_port": 22},
		{"arn": true, "id": true},
	)])
	contains(msg, "admits tcp 22-22")
}

test_egress_rule_passes if {
	count(default_deny_ingress.deny) == 0 with input as plan([change(
		"aws_security_group_rule.egress_all", "aws_security_group_rule",
		{"cidr_blocks": ["0.0.0.0/0"], "from_port": 0, "ipv6_cidr_blocks": null, "prefix_list_ids": null, "protocol": "-1", "to_port": 0, "type": "egress"},
		{"cidr_blocks": [false], "id": true, "source_security_group_id": true},
	)])
}

# The group id is unknown at plan; a group is not a public source.
test_group_references_pass if {
	count(default_deny_ingress.deny) == 0 with input as plan([
		change(
			"aws_security_group.ssh_from_group", "aws_security_group",
			{"ingress": [
				{"cidr_blocks": [], "from_port": 0, "ipv6_cidr_blocks": [], "prefix_list_ids": [], "protocol": "-1", "security_groups": [], "self": true, "to_port": 0},
				{"cidr_blocks": [], "from_port": 22, "ipv6_cidr_blocks": [], "prefix_list_ids": [], "protocol": "tcp", "self": false, "to_port": 22},
			]},
			{"ingress": [
				{"cidr_blocks": [], "ipv6_cidr_blocks": [], "prefix_list_ids": [], "security_groups": []},
				{"cidr_blocks": [], "ipv6_cidr_blocks": [], "prefix_list_ids": [], "security_groups": true},
			]},
		),
		change(
			"aws_vpc_security_group_ingress_rule.ssh_from_group", "aws_vpc_security_group_ingress_rule",
			{"cidr_ipv4": null, "cidr_ipv6": null, "from_port": 22, "ip_protocol": "tcp", "prefix_list_id": null, "to_port": 22},
			{"id": true, "referenced_security_group_id": true},
		),
	])
}

test_unknown_source_denies if {
	some msg in default_deny_ingress.deny with input as plan([change(
		"aws_security_group_rule.open_unknown[\"eip_source\"]", "aws_security_group_rule",
		{"from_port": 22, "ipv6_cidr_blocks": null, "prefix_list_ids": null, "protocol": "tcp", "to_port": 22, "type": "ingress"},
		{"cidr_blocks": true, "id": true, "source_security_group_id": true},
	)])
	contains(msg, "from a cidr_blocks value unknown until apply, which counts as public")
}

test_unknown_rule_type_is_walked if {
	count(default_deny_ingress.deny) == 1 with input as plan([change(
		"aws_security_group_rule.open_unknown_type", "aws_security_group_rule",
		{"cidr_blocks": ["0.0.0.0/0"], "from_port": 22, "ipv6_cidr_blocks": null, "prefix_list_ids": null, "protocol": "tcp", "to_port": 22},
		{"cidr_blocks": [false], "id": true, "source_security_group_id": true, "type": true},
	)])
}

test_unknown_port_denies if {
	some msg in default_deny_ingress.deny with input as plan([change(
		"aws_security_group.open_unknown[\"from_port\"]", "aws_security_group",
		{"ingress": [{"cidr_blocks": ["0.0.0.0/0"], "ipv6_cidr_blocks": [], "prefix_list_ids": [], "protocol": "tcp", "security_groups": [], "self": false, "to_port": 80}]},
		{"ingress": [{"cidr_blocks": [false], "from_port": true, "ipv6_cidr_blocks": [], "prefix_list_ids": [], "security_groups": []}]},
	)])
	contains(msg, "admits a port range unknown until apply")
}

test_unknown_protocol_denies if {
	some msg in default_deny_ingress.deny with input as plan([change(
		"aws_vpc_security_group_ingress_rule.open_unknown[\"protocol\"]", "aws_vpc_security_group_ingress_rule",
		{"cidr_ipv4": "0.0.0.0/0", "cidr_ipv6": null, "from_port": 80, "prefix_list_id": null, "to_port": 80},
		{"id": true, "ip_protocol": true},
	)])
	contains(msg, "admits a protocol unknown until apply")
}

test_prefix_list_denies if {
	default_deny_ingress.deny == {"aws_security_group_rule.open_prefix_list admits prefix list pl-12345678, which is refused on any port because its CIDRs are not in the configuration. Use a literal private CIDR such as 10.0.0.0/16, or a security group"} with input as plan([change(
		"aws_security_group_rule.open_prefix_list", "aws_security_group_rule",
		{"cidr_blocks": null, "from_port": 443, "ipv6_cidr_blocks": null, "prefix_list_ids": ["pl-12345678"], "protocol": "tcp", "to_port": 443, "type": "ingress"},
		{"id": true, "prefix_list_ids": [false], "source_security_group_id": true},
	)])
}

# `ingress = <list unknown until apply>` plans as one unknown value.
test_unknown_ingress_list_denies if {
	default_deny_ingress.deny == {"aws_security_group.open_attribute_collection sets `ingress` from a list that is unknown until apply, so its rules cannot be checked. Write each rule as its own `ingress` block with literal ports and protocol, and cidr_blocks set to a literal private CIDR such as 10.0.0.0/16, or security_groups"} with input as {
		"resource_changes": [change(
			"aws_security_group.open_attribute_collection", "aws_security_group",
			{"name": "open-attribute-collection"},
			{"egress": true, "id": true, "ingress": true},
		)],
		"configuration": {"root_module": {"resources": [{
			"address": "aws_security_group.open_attribute_collection",
			"expressions": {"ingress": {"references": ["local.later_22"]}},
		}]}},
	}
}

# A group that declares no ingress plans the same single unknown value,
# so it passes only when its configuration is found, two modules down.
bare := object.union(
	change(
		"module.child[0].module.grandchild.aws_security_group.bare", "aws_security_group",
		{"name": "grandchild-bare"},
		{"egress": true, "id": true, "ingress": true},
	),
	{"name": "bare", "module_address": "module.child[0].module.grandchild"},
)

test_undeclared_ingress_in_a_module_passes if {
	count(default_deny_ingress.deny) == 0 with input as {
		"resource_changes": [bare],
		"configuration": {"root_module": {"module_calls": {"child": {"module": {
			"resources": [{"address": "aws_security_group.open"}],
			"module_calls": {"grandchild": {"module": {"resources": [{"address": "aws_security_group.bare"}]}}},
		}}}}},
	}
}

test_undeclared_ingress_not_found_denies if {
	count(default_deny_ingress.deny) == 1 with input as {
		"resource_changes": [bare],
		"configuration": {"root_module": {"resources": [{"address": "aws_security_group.bare"}]}},
	}
}

# Trimmed from state_deny_ssh_ipv4.json and state_pass_all_from_peer.json.
web(permissions) := {"group_name": "web", "id": "sg-28d583472ee2a0a7", "ip_permissions": permissions}

permission(protocol, from, to, cidrs, prefix_lists, pairs) := {
	"from_port": from,
	"ip_protocol": protocol,
	"ip_ranges": [{"cidr_ip": cidr, "description": ""} | some cidr in cidrs],
	"ipv6_ranges": [],
	"prefix_list_ids": [{"description": "", "prefix_list_id": id} | some id in prefix_lists],
	"to_port": to,
	"user_id_group_pairs": pairs,
}

state(groups) := {"ec2": {"security_groups": groups}}

test_state_public_ssh_denies if {
	default_deny_ingress.deny_state == {"deployed security group web (sg-28d583472ee2a0a7) admits tcp 22-22, which includes SSH (22), from the public source 0.0.0.0/0"} with input as state([web([permission("tcp", 22, 22, ["0.0.0.0/0"], [], [])])])
}

test_state_prefix_list_denies if {
	default_deny_ingress.deny_state == {"deployed security group web (sg-28d583472ee2a0a7) admits prefix list pl-12345678"} with input as state([web([permission("tcp", 443, 443, [], ["pl-12345678"], [])])])
}

test_state_all_from_a_group_passes if {
	pair := {"description": "", "group_id": "sg-548bd821a56d8963", "user_id": ""}
	count(default_deny_ingress.deny_state) == 0 with input as state([web([permission("-1", 0, 0, [], [], [pair])])])
}

test_state_private_ssh_passes if {
	count(default_deny_ingress.deny_state) == 0 with input as state([web([permission("tcp", 22, 22, ["10.0.0.0/16"], [], [])])])
}

test_state_no_rules_passes if {
	count(default_deny_ingress.deny_state) == 0 with input as state([web([])])
}

test_state_missing_ip_permissions_denies if {
	default_deny_ingress.deny_state == {"deployed security group web (sg-28d583472ee2a0a7) has no ip_permissions in the mock's state, so its ingress cannot be checked"} with input as state([object.remove(web([]), ["ip_permissions"])])
}
