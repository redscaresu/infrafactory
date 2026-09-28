# Fixture for rego_policy_test.go, copied beside the real policies/aws:
# it asserts the opposite of what aws.no_public_db must do, so it fails.
package regotest.failing_test

import rego.v1

# Trimmed from `tofu show -json` of a hashicorp/aws 5.100.0 plan.
public_db := {"planned_values": {"root_module": {"resources": [{
	"address": "aws_db_instance.public",
	"mode": "managed",
	"type": "aws_db_instance",
	"name": "public",
	"provider_name": "registry.opentofu.org/hashicorp/aws",
	"values": {"engine": "postgres", "identifier": "public-db", "publicly_accessible": true},
}]}}}

test_public_db_passes if {
	count(data.aws.no_public_db.deny) == 0 with input as public_db
}
