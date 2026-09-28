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

# Layer 2 — reads the Layer 2 mock's state (fakeaws), never a cloud's
# (ADR-0034 section 4). RDS instance settings live under
# state.rds.instances[].publicly_accessible.
deny_state contains msg if {
	inst := input.rds.instances[_]
	inst.publicly_accessible == true
	msg := sprintf("RDS instance %s is publicly_accessible — must be private", [inst.name])
}
