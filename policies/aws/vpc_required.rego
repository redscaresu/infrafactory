# vpc_required: deny compute / database resources that aren't placed in
# an explicit VPC. AWS supports a default VPC per region but real
# scenarios always create their own — letting the default VPC slip
# through is the most common cause of "default-vpc-not-found" failures
# on accounts that have it disabled.
#
# Per fakeaws/concepts.md "Required surface" item 15 (S43-T11).
package aws.vpc_required

import rego.v1

# EC2 instances must reference a subnet (which by definition belongs to
# a VPC). Skipping subnet_id puts the instance in the default VPC.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_instance"
	not configures(resource, "subnet_id")
	msg := sprintf("%s has no subnet_id — instances MUST be placed in an explicit VPC, not the default", [resource.address])
}

# RDS instances must reference a db_subnet_group_name (which is itself
# subnet-scoped to a custom VPC). Without one, the instance falls back
# to the default VPC.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_db_instance"
	not configures(resource, "db_subnet_group_name")
	msg := sprintf("%s has no db_subnet_group_name — RDS instances MUST be placed in an explicit DB subnet group", [resource.address])
}

# Decided from configuration, not planned_values: both attributes are
# Optional+Computed, so when omitted they are absent from values with
# after_unknown true, exactly like a reference to a subnet not yet
# created. Only the expression tells the two apart. `= null` is an
# omission spelled out, so a null constant does not count, and nor does
# `= var.x` with x null. Any other expression that comes out null (a
# local, a conditional) plans unknown too, and passes.
#
# Configuration holds one entry per block, so the address drops its
# count or for_each index (web[0], web["a"]) to join it.
configures(resource, attr) if {
	block_address := regex.replace(resource.address, `\[.*\]$`, "")
	cfg := input.configuration.root_module.resources[_]
	cfg.address == block_address
	expr := cfg.expressions[attr]
	expr != {"constant_value": null}
	not null_variable(expr)
}

null_variable(expr) if {
	[ref] := expr.references
	startswith(ref, "var.")
	input.variables[trim_prefix(ref, "var.")].value == null
}

# EKS clusters must list subnet_ids in their vpc_config.
# Fires only on known IDs: subnet_ids is a set, and one reference makes
# the whole set unknown at plan time, so `[aws_subnet.a.id]` passes.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_eks_cluster"
	cfg := resource.values.vpc_config[_]
	count(cfg.subnet_ids) < 2
	not eks_subnet_ids_are_unknown(resource)
	msg := sprintf("%s vpc_config.subnet_ids has < 2 subnets — EKS requires ≥2 subnets in different AZs", [resource.address])
}

eks_subnet_ids_are_unknown(resource) if {
	rc := input.resource_changes[_]
	rc.address == resource.address
	cfg := rc.change.after_unknown.vpc_config[_]
	cfg.subnet_ids == true
}

eks_subnet_ids_are_unknown(resource) if {
	rc := input.resource_changes[_]
	rc.address == resource.address
	cfg := rc.change.after_unknown.vpc_config[_]
	count(cfg.subnet_ids) >= 2
}
