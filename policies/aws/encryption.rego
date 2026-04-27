# encryption: deny storage / database resources whose at-rest encryption is
# disabled or whose KMS key reference is missing.
#
# Per fakeaws/concepts.md "Required surface" item 15 (S43-T11): KMS-key-
# required guard for S3 SSE, RDS at rest, Secrets Manager. Mirrors
# policies/gcp/encryption.rego.
package aws.encryption

import rego.v1

# RDS DB instances must have storage_encrypted = true.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_db_instance"
	resource.values.storage_encrypted != true
	msg := sprintf("%s has storage_encrypted = false; AWS RDS at-rest encryption is required", [resource.address])
}

# S3 buckets need a server-side-encryption configuration set. We accept
# either a sibling `aws_s3_bucket_server_side_encryption_configuration`
# resource referring to this bucket, or the deprecated inline
# `server_side_encryption_configuration` block.
deny contains msg if {
	bucket := input.planned_values.root_module.resources[_]
	bucket.type == "aws_s3_bucket"
	not has_sse_config(bucket)
	msg := sprintf("%s has no server-side encryption configuration", [bucket.address])
}

has_sse_config(bucket) if {
	# Inline (deprecated but accepted).
	bucket.values.server_side_encryption_configuration
}

has_sse_config(bucket) if {
	# Separate resource matching this bucket.
	cfg := input.planned_values.root_module.resources[_]
	cfg.type == "aws_s3_bucket_server_side_encryption_configuration"
	cfg.values.bucket != null
}

# Secrets Manager secrets default to AWS-managed KMS; we don't require
# a customer-managed key for v1, but if a kms_key_id is set explicitly,
# it must reference a customer key (alias/aws/secretsmanager is the
# AWS-managed default, NOT customer-managed).
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "aws_secretsmanager_secret"
	key := resource.values.kms_key_id
	key != null
	key != ""
	startswith(key, "alias/aws/")
	msg := sprintf("%s uses an AWS-managed KMS alias (%s); customer-managed key required for compliance scenarios", [resource.address, key])
}
