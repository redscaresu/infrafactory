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
	subnet := resource.values.subnet_id
	subnet == null
	msg := sprintf("%s has no subnet_id — instances MUST be placed in an explicit VPC, not the default", [resource.address])
}

# RDS instances must reference a db_subnet_group_name (which is itself
# subnet-scoped to a custom VPC). Without one, the instance falls back
# to the default VPC.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_db_instance"
	subgrp := resource.values.db_subnet_group_name
	subgrp == null
	msg := sprintf("%s has no db_subnet_group_name — RDS instances MUST be placed in an explicit DB subnet group", [resource.address])
}

# EKS clusters must list subnet_ids in their vpc_config.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_eks_cluster"
	cfg := resource.values.vpc_config[_]
	count(cfg.subnet_ids) < 2
	msg := sprintf("%s vpc_config.subnet_ids has < 2 subnets — EKS requires ≥2 subnets in different AZs", [resource.address])
}
