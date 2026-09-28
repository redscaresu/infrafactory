# Fixture for rego_policy_test.go: a policy under aws/ with no sibling
# policy_test.rego, which the harness must report.
package regotest.untested.aws

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_db_instance"
	resource.values.publicly_accessible == true
	msg := sprintf("%s is publicly accessible", [resource.address])
}
