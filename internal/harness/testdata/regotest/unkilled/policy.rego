# Fixture for rego_policy_test.go: one deny and one deny_state body, only the deny
# body killed by policy_test.rego, so the mutation check must name deny_state.
package regotest.unkilled

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_db_instance"
	resource.values.publicly_accessible == true
	msg := sprintf("%s is publicly accessible", [resource.address])
}

deny_state contains msg if {
	bucket := input.s3.buckets[_]
	bucket.region != input.params.region
	msg := sprintf("S3 bucket %s is in %s", [bucket.name, bucket.region])
}
