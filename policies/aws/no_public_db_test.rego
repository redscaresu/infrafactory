package aws.no_public_db_test

import rego.v1

import data.aws.no_public_db

# Trimmed from `tofu show -json` of a hashicorp/aws 5.100.0 plan.
db(public) := {"planned_values": {"root_module": {"resources": [{
	"address": "aws_db_instance.main",
	"mode": "managed",
	"type": "aws_db_instance",
	"name": "main",
	"provider_name": "registry.opentofu.org/hashicorp/aws",
	"values": {"engine": "postgres", "identifier": "main-db", "publicly_accessible": public},
}]}}}

test_public_db_denies if {
	no_public_db.deny == {"aws_db_instance.main has publicly_accessible = true — RDS instances MUST NOT have public IP addresses"} with input as db(true)
}

test_private_db_passes if {
	count(no_public_db.deny) == 0 with input as db(false)
}
