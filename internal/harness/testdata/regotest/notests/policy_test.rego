# Fixture for rego_policy_test.go: a _test.rego with no test_ rule
# checks nothing, and the harness must say so.
package regotest.notests_test

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
