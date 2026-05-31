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
# M98: also pass when subnet_id is a known-after-apply reference.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_instance"
	subnet := resource.values.subnet_id
	subnet == null
	not subnet_id_is_reference(resource)
	msg := sprintf("%s has no subnet_id — instances MUST be placed in an explicit VPC, not the default", [resource.address])
}

subnet_id_is_reference(resource) if {
	rc := input.resource_changes[_]
	rc.address == resource.address
	rc.change.after_unknown.subnet_id == true
}

# RDS instances must reference a db_subnet_group_name (which is itself
# subnet-scoped to a custom VPC). Without one, the instance falls back
# to the default VPC.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_db_instance"
	subgrp := resource.values.db_subnet_group_name
	subgrp == null
	not db_subnet_group_is_reference(resource)
	msg := sprintf("%s has no db_subnet_group_name — RDS instances MUST be placed in an explicit DB subnet group", [resource.address])
}

db_subnet_group_is_reference(resource) if {
	rc := input.resource_changes[_]
	rc.address == resource.address
	rc.change.after_unknown.db_subnet_group_name == true
}

# EKS clusters must list subnet_ids in their vpc_config.
# M98: subnet_ids may be a list of references — when entirely unknown,
# `after_unknown.vpc_config[0].subnet_ids` is `true` (whole list
# unknown) or a list of true values; either way pass.
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
