# no_public_db: RDS instances must not be publicly accessible.
#
# Per fakeaws/concepts.md "Required surface" item 15 (S43-T11). Mirrors
# policies/gcp/no_public_sql.rego — same intent, AWS-specific shape.
package aws.no_public_db

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_db_instance"
	resource.values.publicly_accessible == true
	msg := sprintf("%s has publicly_accessible = true — RDS instances MUST NOT have public IP addresses", [resource.address])
}

# No deny_state, and why: internal/harness/aws_no_public_db_test.go.
