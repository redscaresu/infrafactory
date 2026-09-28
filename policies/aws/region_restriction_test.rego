package aws.region_restriction_test

import rego.v1

import data.aws.region_restriction

params := {"region": "us-east-1"}

# Trimmed from `tofu show -json` of a hashicorp/aws 5.100.0 plan:
# `region = var.region` plans as references, a literal as constant_value,
# and a block with no region argument has no region key at all.
var_region := {"references": ["var.region"]}

literal_region(region) := {"constant_value": region}

plan(region_expr, az) := {
	"configuration": {"provider_config": {"aws": {
		"name": "aws",
		"full_name": "registry.opentofu.org/hashicorp/aws",
		"expressions": object.union(
			{"skip_credentials_validation": {"constant_value": true}},
			region_expr,
		),
	}}},
	"planned_values": {"root_module": {"resources": [
		{
			"address": "aws_sqs_queue.q",
			"mode": "managed",
			"type": "aws_sqs_queue",
			"name": "q",
			"provider_name": "registry.opentofu.org/hashicorp/aws",
			"values": {"name": "q", "fifo_queue": false},
		},
		{
			"address": "aws_subnet.a",
			"mode": "managed",
			"type": "aws_subnet",
			"name": "a",
			"provider_name": "registry.opentofu.org/hashicorp/aws",
			"values": {"availability_zone": az, "cidr_block": "10.0.1.0/24"},
		},
	]}},
	"params": params,
}

test_provider_in_another_region_denies if {
	region_restriction.deny == {"provider aws is in region eu-west-1 — must be in us-east-1"} with input as plan({"region": literal_region("eu-west-1")}, "us-east-1a")
}

test_provider_region_from_a_variable_denies if {
	region_restriction.deny == {`provider aws sets no literal region, so the plan cannot tell where it deploys — set region = "us-east-1"`} with input as plan({"region": var_region}, "us-east-1a")
}

test_provider_without_region_denies if {
	region_restriction.deny == {`provider aws sets no literal region, so the plan cannot tell where it deploys — set region = "us-east-1"`} with input as plan({}, "us-east-1a")
}

test_aws_resources_without_aws_provider_deny if {
	no_provider := json.remove(plan({"region": literal_region("us-east-1")}, "us-east-1a"), ["configuration"])
	region_restriction.deny == {`aws resources are planned with no aws provider configuration, so their region cannot be read — declare provider "aws" with region = "us-east-1"`} with input as no_provider
}

test_literal_region_and_zone_in_region_pass if {
	count(region_restriction.deny) == 0 with input as plan({"region": literal_region("us-east-1")}, "us-east-1a")
}

test_zone_in_another_region_denies if {
	region_restriction.deny == {"aws_subnet.a is in availability_zone eu-west-1a — must be in us-east-1"} with input as plan({"region": literal_region("us-east-1")}, "eu-west-1a")
}

# Trimmed from fakeaws GET /mock/state after CreateQueue and CreateSubnet
# on /region/<region>; params is what infrafactory adds from the criterion.
# Global services record no region.
mock_state(region, az) := {
	"schema_version": 1,
	"audit": [],
	"iam": {"roles": [{"arn": "arn:aws:iam::000000000000:role/r", "name": "r", "path": "/"}]},
	"sqs": {
		"messages": [],
		"queues": [{
			"arn": sprintf("arn:aws:sqs:%s:000000000000:orders", [region]),
			"fifo": false,
			"name": "orders",
			"region": region,
			"url": "http://127.0.0.1:8082/000000000000/orders",
		}],
		"tombstoned_messages": 0,
	},
	"ec2": {"subnets": [{
		"arn": sprintf("arn:aws:ec2:%s:000000000000:subnet/subnet-05cde77bf3b9f19f", [region]),
		"availability_zone": az,
		"cidr_block": "10.0.1.0/24",
		"id": "subnet-05cde77bf3b9f19f",
		"region": region,
		"vpc_id": "vpc-d4c1761754103ed1",
	}]},
	"params": params,
}

test_state_outside_region_denies if {
	region_restriction.deny_state == {
		"orders is in region eu-west-1 in the deployed state — must be in us-east-1",
		"subnet-05cde77bf3b9f19f is in region eu-west-1 in the deployed state — must be in us-east-1",
		"subnet-05cde77bf3b9f19f is in availability_zone eu-west-1a in the deployed state — must be in us-east-1",
	} with input as mock_state("eu-west-1", "eu-west-1a")
}

test_state_in_region_passes if {
	count(region_restriction.deny_state) == 0 with input as mock_state("us-east-1", "us-east-1a")
}

test_no_params_region_denies_nothing if {
	count(region_restriction.deny) == 0 with input as json.remove(plan({"region": var_region}, "eu-west-1a"), ["params"])
	count(region_restriction.deny_state) == 0 with input as json.remove(mock_state("eu-west-1", "eu-west-1a"), ["params"])
}
