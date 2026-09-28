package aws.default_deny_ingress

import rego.v1

# No security group opens SSH or all traffic to a public source, and none
# admits a prefix list (ADR-0034).
#
# Four shapes open ingress, and a walk that misses one fails open on it:
# inline `ingress` on aws_security_group and on aws_default_security_group,
# aws_security_group_rule with type = "ingress", and
# aws_vpc_security_group_ingress_rule. Egress is never walked.
#
# The walk is over resource_changes, not planned_values.root_module:
# resource_changes is flat across modules, so a group inside a child
# module is walked, and change.after_unknown is where unknown values are.
#
# A source is public unless it is inside 10/8, 172.16/12, 192.168/16 or
# fc00::/7, so ::/0, the two halves of 0.0.0.0/0 and any public /32 are
# all public. A security-group reference (security_groups, self,
# source_security_group_id, referenced_security_group_id) is not a source
# here at all. A prefix list is refused on any port: its CIDRs are not in
# the configuration.
#
# Unknowns fail closed: an unknown source field counts as public, and an
# unknown protocol or port as admitting 22.
#
# Denial text is repair-loop INPUT, recorded verbatim as a pitfall, so it
# names the address, the rule and the fix.
deny contains msg if {
	some rule in rules
	some field in rule.source_fields
	some source in public_sources(rule, field)
	opens := opening(rule)
	msg := sprintf(
		"%s admits %s from %s. Open only the ports the scenario needs to the internet, such as tcp 80, and admit SSH or all traffic only from a literal private CIDR such as 10.0.0.0/16, or from a security group",
		[rule.address, opens, source],
	)
}

deny contains msg if {
	some rule in rules
	some prefix_list in prefix_lists(rule)
	msg := sprintf(
		"%s admits prefix list %s, which is refused on any port because its CIDRs are not in the configuration. Use a literal private CIDR such as 10.0.0.0/16, or a security group",
		[rule.address, prefix_list],
	)
}

# The one case where the plan holds no rules to judge: `ingress` set by
# an expression whose list is unknown until apply comes out as a single
# unknown value. A dynamic block does not: over an unknown for_each it
# plans as one rule with every field unknown, which the rules above deny.
#
# A group that declares no `ingress` also plans as a single unknown value,
# because the attribute is computed, so the configuration decides:
# anything but a found block with no `ingress` expression denies.
deny contains msg if {
	some change in changes
	change.type in inline_types
	change.change.after_unknown.ingress == true
	not omits_ingress(change)
	msg := sprintf(
		"%s sets `ingress` from a list that is unknown until apply, so its rules cannot be checked. Write each rule as its own `ingress` block with literal ports and protocol, and cidr_blocks set to a literal private CIDR such as 10.0.0.0/16, or security_groups",
		[change.address],
	)
}

# Same prohibition against the Layer 2 mock's state after the mock apply.
# fakeaws folds all four shapes into the group's ip_permissions. It reads
# the mock's record, never a cloud's, even on a run that also applied to
# a sandbox: the holdout is what checks the deployed stack (ADR-0034
# section 4).
deny_state contains msg if {
	some group in input.ec2.security_groups
	some permission in group.ip_permissions
	some source in state_public_sources(permission)
	opens := opening({"protocol": permission.ip_protocol, "from": permission.from_port, "to": permission.to_port, "unknown": {}})
	msg := sprintf("deployed security group %v (%v) admits %s from %s", [group.group_name, group.id, opens, source])
}

deny_state contains msg if {
	some group in input.ec2.security_groups
	some permission in group.ip_permissions
	some prefix_list in permission.prefix_list_ids
	msg := sprintf("deployed security group %v (%v) admits prefix list %v", [group.group_name, group.id, prefix_list.prefix_list_id])
}

# Helper plus `not`: a group with no ip_permissions key would otherwise
# walk nothing and read as a group with no rules. [] passes.
deny_state contains msg if {
	some group in input.ec2.security_groups
	not has_ip_permissions(group)
	msg := sprintf("deployed security group %v (%v) has no ip_permissions in the mock's state, so its ingress cannot be checked", [group.group_name, group.id])
}

has_ip_permissions(group) if is_array(group.ip_permissions)

inline_types := {"aws_security_group", "aws_default_security_group"}

private_ranges := ["10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"]

changes contains change if {
	some change in input.resource_changes
	change.mode == "managed"
	change.change.actions != ["delete"]
}

# Every ingress rule in the plan, in one form: the values the plan knows
# and the matching after_unknown, with each shape's field names.
rules contains rule if {
	some change in changes
	change.type in inline_types
	some i, block in change.change.after.ingress
	rule := normalize(change.address, as_object(block), element_unknown(change, i), list_fields)
}

# `not ... == "egress"`, so a type unknown until apply is walked.
rules contains rule if {
	some change in changes
	change.type == "aws_security_group_rule"
	not change.change.after.type == "egress"
	rule := normalize(change.address, change.change.after, change.change.after_unknown, list_fields)
}

rules contains rule if {
	some change in changes
	change.type == "aws_vpc_security_group_ingress_rule"
	rule := normalize(change.address, change.change.after, change.change.after_unknown, vpc_rule_fields)
}

list_fields := {"protocol": "protocol", "sources": ["cidr_blocks", "ipv6_cidr_blocks"], "prefix_lists": "prefix_list_ids"}

vpc_rule_fields := {"protocol": "ip_protocol", "sources": ["cidr_ipv4", "cidr_ipv6"], "prefix_lists": "prefix_list_id"}

# An unknown value is absent from `after`, so it reads as null here and
# after_unknown says which it was: null ports are real on an
# aws_vpc_security_group_ingress_rule for ICMP or protocol -1.
normalize(address, values, unknown, fields) := {
	"address": address,
	"values": values,
	"unknown": unknown,
	"protocol": object.get(values, fields.protocol, null),
	"from": object.get(values, "from_port", null),
	"to": object.get(values, "to_port", null),
	"source_fields": fields.sources,
	"prefix_field": fields.prefix_lists,
}

# jsonplan renders a wholly unknown element as null. No capture yields
# one, as an unknown element makes the whole set unknown, but a null here
# would otherwise drop the rule rather than deny it.
as_object(block) := block if {
	is_object(block)
} else := {}

element_unknown(change, i) := unknown if {
	unknown := change.change.after_unknown.ingress[i]
} else := {}

# walk() yields a string field itself and each string in a list field, so
# one expression reads cidr_ipv4 and cidr_blocks alike.
public_sources(rule, field) := {source |
	walk(object.get(rule.values, field, null), [_, cidr])
	is_string(cidr)
	not private(cidr)
	source := sprintf("the public source %s", [cidr])
} | {source |
	unknown(rule.unknown, field)
	source := sprintf("a %s value unknown until apply, which counts as public", [field])
}

prefix_lists(rule) := {prefix_list |
	walk(object.get(rule.values, rule.prefix_field, null), [_, prefix_list])
	is_string(prefix_list)
} | {prefix_list |
	unknown(rule.unknown, rule.prefix_field)
	prefix_list := sprintf("%s (unknown until apply)", [rule.prefix_field])
}

state_public_sources(permission) := {sprintf("the public source %s", [range.cidr_ip]) |
	some range in permission.ip_ranges
	not private(range.cidr_ip)
} | {sprintf("the public source %s", [range.cidr_ipv6]) |
	some range in permission.ipv6_ranges
	not private(range.cidr_ipv6)
}

# A malformed CIDR makes net.cidr_contains undefined, so it is public.
private(cidr) if {
	some range in private_ranges
	net.cidr_contains(range, cidr)
}

unknown(u, _) if u == true

unknown(u, field) if {
	walk(u[field], [_, value])
	value == true
}

# What a public rule opens, or undefined when it does not reach SSH.
# The protocol is required on every shape, so a missing one is unknown.
opening(rule) := "a protocol unknown until apply" if {
	not is_string(rule.protocol)
} else := sprintf("all traffic (protocol %s)", [rule.protocol]) if {
	lower(rule.protocol) in {"-1", "all"}
} else := "a port range unknown until apply" if {
	some field in ["from_port", "to_port"]
	unknown(rule.unknown, field)
} else := sprintf("ports 0-0 (protocol %s)", [rule.protocol]) if {
	rule.from == 0
	rule.to == 0
} else := sprintf("tcp %d-%d, which includes SSH (22),", [rule.from, rule.to]) if {
	lower(rule.protocol) in {"tcp", "6"}
	rule.from <= 22
	rule.to >= 22
}

# Found in the configuration, at the change's module path, with no
# `ingress` expression. The module path drops instance keys
# (module.child[0] is module_calls.child), as configuration holds one
# entry per call.
omits_ingress(change) if {
	walk(input.configuration.root_module, [path, resource])
	path[count(path) - 2] == "resources"
	resource.address == sprintf("%s.%s", [change.type, change.name])
	module_calls(path) == change_module_calls(change)
	not resource.expressions.ingress
}

module_calls(path) := [path[i + 1] | some i, key in path; key == "module_calls"]

# [] at the root, where a change has no module_address.
change_module_calls(change) := [call |
	some i, call in split(regex.replace(change.module_address, `\[[^\]]*\]`, ""), ".")
	i % 2 == 1
]
