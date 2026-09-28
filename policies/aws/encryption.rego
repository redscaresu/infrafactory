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

# S3 buckets need a server-side-encryption configuration of their own:
# the deprecated inline `server_side_encryption_configuration` block, or
# an `aws_s3_bucket_server_side_encryption_configuration` naming this
# bucket. A configuration for another bucket does not count.
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

# Named by reference. `bucket = aws_s3_bucket.a.id` is unknown at plan
# time and absent from planned_values (M98), so the reference is read
# from configuration. The config must also have a planned instance:
# configuration still lists one with `count = 0`.
has_sse_config(bucket) if {
	cfg := input.configuration.root_module.resources[_]
	cfg.type == "aws_s3_bucket_server_side_encryption_configuration"
	instance := input.planned_values.root_module.resources[_]
	instance.type == cfg.type
	instance.name == cfg.name
	names_bucket(cfg.expressions.bucket.references, instance, bucket)
}

# Named by the bucket's literal name.
has_sse_config(bucket) if {
	cfg := input.planned_values.root_module.resources[_]
	cfg.type == "aws_s3_bucket_server_side_encryption_configuration"
	cfg.values.bucket == bucket.values.bucket
}

# `aws_s3_bucket.a.id` records "aws_s3_bucket.a", and
# `aws_s3_bucket.x[0].id` records "aws_s3_bucket.x[0]": the bucket's own
# address.
names_bucket(refs, _, bucket) if bucket.address in refs

# `aws_s3_bucket.x[count.index].id` (or `[each.key]`) records only the
# bare "aws_s3_bucket.x", so the config instance names the bucket with
# its own index. This assumes the two indexes line up, which
# `x[count.index + 1]` would break.
names_bucket(refs, instance, bucket) if {
	bare := sprintf("%s.%s", [bucket.type, bucket.name])
	bare in refs
	not indexed_ref(refs, bare)
	instance.index == bucket.index
}

indexed_ref(refs, bare) if startswith(refs[_], concat("", [bare, "["]))

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
