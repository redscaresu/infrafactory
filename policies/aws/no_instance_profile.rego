# no_instance_profile: no instance launches with an IAM instance profile.
#
# The web stack's instance runs with no instance profile
# (docs/hld/2026-09-27-aws-web-stack.md § Identity). This runs on every
# AWS plan and needs no criterion; the Layer 3 gate refuses the same
# attribute only at Layer 3.
#
# Walks every resource type that launches an instance with a profile, at
# any module depth: a walk that misses one fails open on it (ADR-0034).
# On aws_instance the attribute is Optional+Computed, so after_unknown is
# true on every instance that omits it, and unknown values are judged from
# the configuration instead: an argument that is set at all is denied.
package aws.no_instance_profile

import rego.v1

# The types whose iam_instance_profile is a string argument.
string_profile_types := {"aws_instance", "aws_spot_instance_request", "aws_launch_configuration"}

profile_types := string_profile_types | {"aws_launch_template"}

message(address) := sprintf("%s sets iam_instance_profile — instances MUST NOT launch with an instance profile; remove the iam_instance_profile argument or block, and any aws_iam_instance_profile created for it", [address])

# A known profile name, including one prior state carries while the
# configuration omits it.
deny contains message(rc.address) if {
	rc := input.resource_changes[_]
	rc.mode == "managed"
	string_profile_types[rc.type]
	is_string(rc.change.after.iam_instance_profile)
	rc.change.after.iam_instance_profile != ""
}

# A launch template block, including a dynamic block the configuration
# does not list and one whose count is unknown at plan.
deny contains message(rc.address) if {
	rc := input.resource_changes[_]
	rc.mode == "managed"
	rc.type == "aws_launch_template"
	launch_template_has_profile(rc.change)
}

launch_template_has_profile(change) if count(change.after.iam_instance_profile) > 0

launch_template_has_profile(change) if change.after_unknown.iam_instance_profile == true

# Any profile argument or block in the configuration, at any module depth,
# which is where a reference that is unknown at plan still shows.
deny contains message(concat("", [module_prefix(path), resource.address])) if {
	walk(input.configuration, [path, resource])
	count(path) > 1
	path[count(path) - 2] == "resources"
	resource.mode == "managed"
	profile_types[resource.type]
	resource.expressions.iam_instance_profile
}

# module_prefix turns a configuration path such as
# ["root_module", "module_calls", "web", "module", "resources", 0]
# into "module.web.".
module_prefix(path) := concat("", [sprintf("module.%s.", [path[i + 1]]) | path[i] == "module_calls"])
