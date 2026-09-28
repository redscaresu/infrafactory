# Fixture for rego_policy_test.go: a policy under gcp/ with no sibling
# policy_test.rego, which the harness must not report (GCP is out of scope).
package regotest.untested.gcp

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "google_sql_database_instance"
	resource.values.settings[_].ip_configuration[_].ipv4_enabled == true
	msg := sprintf("%s has a public IP", [resource.address])
}
