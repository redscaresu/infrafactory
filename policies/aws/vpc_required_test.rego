package aws.vpc_required_test

import rego.v1

import data.aws.vpc_required

# Trimmed from `tofu show -json` of a hashicorp/aws 5.100.0 plan. Omitted
# or referenced, subnet_id and db_subnet_group_name are absent from values
# with after_unknown true; only configuration.expressions differs.
plan(block, address, attr, expressions) := {
	"planned_values": {"root_module": {"resources": [{
		"address": address,
		"mode": "managed",
		"type": split(block, ".")[0],
		"values": {},
	}]}},
	"resource_changes": [{
		"address": address,
		"type": split(block, ".")[0],
		"change": {"after_unknown": {attr: true}},
	}],
	"configuration": {"root_module": {"resources": [{
		"address": block,
		"mode": "managed",
		"type": split(block, ".")[0],
		"expressions": expressions,
	}]}},
}

subnet_ref := {"subnet_id": {"references": ["aws_subnet.main.id", "aws_subnet.main"]}}

subnet_group_ref := {"db_subnet_group_name": {"references": ["aws_db_subnet_group.main.name", "aws_db_subnet_group.main"]}}

no_subnet(address) := sprintf("%s has no subnet_id — instances MUST be placed in an explicit VPC, not the default", [address])

no_subnet_group(address) := sprintf("%s has no db_subnet_group_name — RDS instances MUST be placed in an explicit DB subnet group", [address])

test_instance_omitted_subnet_denies if {
	vpc_required.deny == {no_subnet("aws_instance.web")} with input as plan("aws_instance.web", "aws_instance.web", "subnet_id", {"ami": {"constant_value": "ami-12345678"}})
}

test_instance_null_subnet_denies if {
	vpc_required.deny == {no_subnet("aws_instance.web")} with input as plan("aws_instance.web", "aws_instance.web", "subnet_id", {"subnet_id": {"constant_value": null}})
}

subnet_from_variable(value) := object.union(
	plan("aws_instance.web", "aws_instance.web", "subnet_id", {"subnet_id": {"references": ["var.subnet_id"]}}),
	{"variables": {"subnet_id": {"value": value}}},
)

test_instance_subnet_from_variable_denies_only_when_null if {
	vpc_required.deny == {no_subnet("aws_instance.web")} with input as subnet_from_variable(null)
	count(vpc_required.deny) == 0 with input as subnet_from_variable("subnet-0123456789abcdef0")
}

test_instance_literal_subnet_passes if {
	count(vpc_required.deny) == 0 with input as plan("aws_instance.web", "aws_instance.web", "subnet_id", {"subnet_id": {"constant_value": "subnet-0123456789abcdef0"}})
}

test_indexed_instance_with_subnet_reference_passes if {
	count(vpc_required.deny) == 0 with input as plan("aws_instance.web", "aws_instance.web[0]", "subnet_id", subnet_ref)
	count(vpc_required.deny) == 0 with input as plan("aws_instance.web", `aws_instance.web["a"]`, "subnet_id", subnet_ref)
}

test_indexed_instance_without_subnet_denies if {
	vpc_required.deny == {no_subnet("aws_instance.web[0]")} with input as plan("aws_instance.web", "aws_instance.web[0]", "subnet_id", {})
}

test_db_instance_omitted_subnet_group_denies if {
	vpc_required.deny == {no_subnet_group("aws_db_instance.db")} with input as plan("aws_db_instance.db", "aws_db_instance.db", "db_subnet_group_name", {"engine": {"constant_value": "postgres"}})
}

test_indexed_db_instance_with_subnet_group_reference_passes if {
	count(vpc_required.deny) == 0 with input as plan("aws_db_instance.db", "aws_db_instance.db[0]", "db_subnet_group_name", subnet_group_ref)
}

test_indexed_db_instance_without_subnet_group_denies if {
	vpc_required.deny == {no_subnet_group("aws_db_instance.db[0]")} with input as plan("aws_db_instance.db", "aws_db_instance.db[0]", "db_subnet_group_name", {})
}

eks(subnet_ids_value, subnet_ids_unknown) := {
	"planned_values": {"root_module": {"resources": [{
		"address": "aws_eks_cluster.k8s",
		"mode": "managed",
		"type": "aws_eks_cluster",
		"values": {"vpc_config": [object.union({"endpoint_public_access": true}, subnet_ids_value)]},
	}]}},
	"resource_changes": [{
		"address": "aws_eks_cluster.k8s",
		"type": "aws_eks_cluster",
		"change": {"after_unknown": {"vpc_config": [{"subnet_ids": subnet_ids_unknown, "vpc_id": true}]}},
	}],
}

test_eks_one_literal_subnet_denies if {
	vpc_required.deny == {"aws_eks_cluster.k8s vpc_config.subnet_ids has < 2 subnets — EKS requires ≥2 subnets in different AZs"} with input as eks({"subnet_ids": ["subnet-0123456789abcdef0"]}, [false])
}

# The stated limit: one reference makes the whole set unknown, so the
# count cannot be checked at plan time.
test_eks_one_reference_passes if {
	count(vpc_required.deny) == 0 with input as eks({}, true)
}
