package aws.no_instance_profile_test

import rego.v1

import data.aws.no_instance_profile

# Trimmed from `tofu show -json` of hashicorp/aws 5.100.0 plans captured
# against fakeaws (internal/harness/testdata/instance-profile/aws/).

change(address, type, after, after_unknown) := {
	"address": address,
	"mode": "managed",
	"type": type,
	"change": {"actions": ["create"], "after": after, "after_unknown": after_unknown},
}

config(resources) := {"root_module": {"resources": resources}}

config_resource(address, type, expressions) := {
	"address": address,
	"mode": "managed",
	"type": type,
	"expressions": expressions,
}

denied(address) := {sprintf("%s sets iam_instance_profile — instances MUST NOT launch with an instance profile; remove the iam_instance_profile argument or block, and any aws_iam_instance_profile created for it", [address])}

# A known name with no configuration: what an instance whose prior state
# carries a profile plans when the configuration omits the argument.
test_known_profile_name_denies if {
	no_instance_profile.deny == denied("aws_instance.web") with input as {"resource_changes": [change("aws_instance.web", "aws_instance", {"iam_instance_profile": "web-profile"}, {})]}
}

test_empty_profile_name_passes if {
	count(no_instance_profile.deny) == 0 with input as {"resource_changes": [change("aws_instance.web", "aws_instance", {"iam_instance_profile": ""}, {})]}
}

test_literal_profile_name_denies_once if {
	no_instance_profile.deny == denied("aws_instance.web") with input as {
		"resource_changes": [change("aws_instance.web", "aws_instance", {"iam_instance_profile": "web-profile"}, {})],
		"configuration": config([config_resource("aws_instance.web", "aws_instance", {"iam_instance_profile": {"constant_value": "web-profile"}})]),
	}
}

unknown_profile_reference := {"iam_instance_profile": {"references": ["aws_iam_instance_profile.web.name", "aws_iam_instance_profile.web"]}}

test_unknown_profile_name_denies if {
	no_instance_profile.deny == denied("aws_instance.web") with input as {
		"resource_changes": [change("aws_instance.web", "aws_instance", {}, {"iam_instance_profile": true})],
		"configuration": config([config_resource("aws_instance.web", "aws_instance", unknown_profile_reference)]),
	}
}

test_unknown_profile_name_in_child_module_denies if {
	no_instance_profile.deny == denied("module.web.aws_instance.web") with input as {
		"resource_changes": [change("module.web.aws_instance.web", "aws_instance", {}, {"iam_instance_profile": true})],
		"configuration": {"root_module": {"module_calls": {"web": {
			"source": "./child",
			"module": {"resources": [config_resource("aws_instance.web", "aws_instance", unknown_profile_reference)]},
		}}}},
	}
}

test_nested_child_module_denies if {
	no_instance_profile.deny == denied("module.a.module.b.aws_instance.web") with input as {"configuration": {"root_module": {"module_calls": {"a": {"module": {"module_calls": {"b": {"module": {"resources": [config_resource("aws_instance.web", "aws_instance", unknown_profile_reference)]}}}}}}}}}
}

test_spot_instance_and_launch_configuration_deny if {
	no_instance_profile.deny == denied("aws_spot_instance_request.web") | denied("aws_launch_configuration.web") with input as {"configuration": config([
		config_resource("aws_spot_instance_request.web", "aws_spot_instance_request", {"iam_instance_profile": {"constant_value": "web-profile"}}),
		config_resource("aws_launch_configuration.web", "aws_launch_configuration", {"iam_instance_profile": {"constant_value": "web-profile"}}),
	])}
}

# The web-step-one instance: Optional+Computed makes the unset argument
# unknown at plan, and the configuration does not set it.
test_instance_without_profile_passes if {
	count(no_instance_profile.deny) == 0 with input as {
		"resource_changes": [change("aws_instance.web", "aws_instance", {"ami": "ami-0al2023x8664"}, {"iam_instance_profile": true})],
		"configuration": config([config_resource("aws_instance.web", "aws_instance", {"ami": {"constant_value": "ami-0al2023x8664"}})]),
	}
}

test_launch_template_block_denies if {
	no_instance_profile.deny == denied("aws_launch_template.web") with input as {
		"resource_changes": [change("aws_launch_template.web", "aws_launch_template", {"iam_instance_profile": [{"arn": null, "name": "web-profile"}]}, {"iam_instance_profile": [{}]})],
		"configuration": config([config_resource("aws_launch_template.web", "aws_launch_template", {"iam_instance_profile": [{"name": {"constant_value": "web-profile"}}]})]),
	}
}

# A dynamic block is absent from the configuration's expressions.
test_launch_template_dynamic_block_denies if {
	no_instance_profile.deny == denied("aws_launch_template.web") with input as {
		"resource_changes": [change("aws_launch_template.web", "aws_launch_template", {"iam_instance_profile": [{"arn": null}]}, {"iam_instance_profile": [{"name": true}]})],
		"configuration": config([config_resource("aws_launch_template.web", "aws_launch_template", {})]),
	}
}

test_launch_template_unknown_block_count_denies if {
	no_instance_profile.deny == denied("aws_launch_template.web") with input as {
		"resource_changes": [change("aws_launch_template.web", "aws_launch_template", {}, {"iam_instance_profile": true})],
		"configuration": config([config_resource("aws_launch_template.web", "aws_launch_template", {})]),
	}
}

test_launch_template_without_profile_passes if {
	count(no_instance_profile.deny) == 0 with input as {
		"resource_changes": [change("aws_launch_template.web", "aws_launch_template", {"iam_instance_profile": []}, {"iam_instance_profile": []})],
		"configuration": config([config_resource("aws_launch_template.web", "aws_launch_template", {"image_id": {"constant_value": "ami-0al2023x8664"}})]),
	}
}

test_deleted_instance_passes if {
	count(no_instance_profile.deny) == 0 with input as {"resource_changes": [{
		"address": "aws_instance.web",
		"mode": "managed",
		"type": "aws_instance",
		"change": {"actions": ["delete"], "before": {"iam_instance_profile": "web-profile"}, "after": null, "after_unknown": {}},
	}]}
}
