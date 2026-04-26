# encryption: deny storage / database / disk resources that are not encrypted at rest.
# Targets:
#   google_storage_bucket          — must declare encryption.default_kms_key_name (CMEK).
#   google_sql_database_instance   — must declare encryption_key_name on the instance.
#   google_compute_disk            — must declare disk_encryption_key.kms_key_self_link
#                                    (CMEK) or disk_encryption_key.sha256 (Google-managed
#                                    customer-supplied key).
# Google encrypts all of these by default, but this policy enforces an explicit
# customer-managed (CMEK) configuration so encryption posture is auditable in HCL.
package gcp.encryption

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "google_storage_bucket"
	not bucket_has_cmek(resource)
	msg := sprintf(
		"%s has no encryption.default_kms_key_name — customer-managed encryption not configured",
		[resource.address],
	)
}

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "google_sql_database_instance"
	not sql_has_cmek(resource)
	msg := sprintf(
		"%s has no encryption_key_name — customer-managed encryption not configured",
		[resource.address],
	)
}

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "google_compute_disk"
	not disk_has_encryption(resource)
	msg := sprintf(
		"%s has no disk_encryption_key — customer-managed encryption not configured",
		[resource.address],
	)
}

bucket_has_cmek(resource) if {
	enc := resource.values.encryption[_]
	enc.default_kms_key_name != null
	enc.default_kms_key_name != ""
}

sql_has_cmek(resource) if {
	resource.values.encryption_key_name != null
	resource.values.encryption_key_name != ""
}

disk_has_encryption(resource) if {
	enc := resource.values.disk_encryption_key[_]
	enc.kms_key_self_link != null
	enc.kms_key_self_link != ""
}

disk_has_encryption(resource) if {
	enc := resource.values.disk_encryption_key[_]
	enc.sha256 != null
	enc.sha256 != ""
}
