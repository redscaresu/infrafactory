package aws.encryption_test

import rego.v1

import data.aws.encryption

# Fixtures trimmed from `tofu show -json` of a hashicorp/aws 5.100.0 plan.
plan(planned, changes, config) := {
	"planned_values": {"root_module": {"resources": planned}},
	"resource_changes": changes,
	"configuration": {"root_module": {"resources": config}},
}

resource(type, name, values) := {
	"address": sprintf("%s.%s", [type, name]),
	"mode": "managed",
	"type": type,
	"name": name,
	"values": values,
}

# A counted or for_each instance: `aws_s3_bucket.x[0]`, `aws_s3_bucket.f["a"]`.
instance(r, index) := object.union(r, {"address": sprintf("%s[%s]", [r.address, json.marshal(index)]), "index": index})

sse := "aws_s3_bucket_server_side_encryption_configuration"

sse_config(name, bucket) := {
	"address": sprintf("%s.%s", [sse, name]),
	"mode": "managed",
	"type": sse,
	"name": name,
	"expressions": {"bucket": bucket},
}

sse_rule := [{"apply_server_side_encryption_by_default": [{"kms_master_key_id": null, "sse_algorithm": "AES256"}], "bucket_key_enabled": null}]

# `bucket = aws_s3_bucket.a.id`: unknown at plan, so planned_values omits
# it and only configuration says which bucket it names.
test_sse_config_for_another_bucket_denies if {
	encryption.deny == {"aws_s3_bucket.b has no server-side encryption configuration"} with input as plan(
		[
			resource("aws_s3_bucket", "a", {"bucket": "logs-a"}),
			resource("aws_s3_bucket", "b", {"bucket": "logs-b"}),
			resource(sse, "a", {"rule": sse_rule}),
		],
		[{
			"address": "aws_s3_bucket_server_side_encryption_configuration.a",
			"type": sse,
			"name": "a",
			"change": {"actions": ["create"], "after_unknown": {"bucket": true}},
		}],
		[sse_config("a", {"references": ["aws_s3_bucket.a.id", "aws_s3_bucket.a"]})],
	)
}

test_sse_config_naming_bucket_literally_passes if {
	count(encryption.deny) == 0 with input as plan(
		[
			resource("aws_s3_bucket", "b", {"bucket": "logs-b"}),
			resource(sse, "b", {"bucket": "logs-b", "rule": sse_rule}),
		],
		[],
		[sse_config("b", {"constant_value": "logs-b"})],
	)
}

# `count = 1` on both; `bucket = aws_s3_bucket.x[count.index].id` records
# only the bare resource and count.index.
test_counted_bucket_with_sse_config_referencing_it_passes if {
	count(encryption.deny) == 0 with input as plan(
		[
			instance(resource("aws_s3_bucket", "x", {}), 0),
			instance(resource(sse, "x", {"rule": sse_rule}), 0),
		],
		[],
		[sse_config("x", {"references": ["aws_s3_bucket.x", "count.index"]})],
	)
}

# x: count = 2 buckets, count = 1 config on `aws_s3_bucket.x[count.index].id`.
# y: count = 2 buckets, count = 1 config on `aws_s3_bucket.y[1].id`; its
# instance index 0 is not the bucket it names.
# f: for_each a and b; a config for_each ["a"] on `aws_s3_bucket.f[each.key].id`
# and one on `aws_s3_bucket.f["b"].id`.
test_indexed_sse_config_binds_only_its_instance if {
	encryption.deny == {
		"aws_s3_bucket.x[1] has no server-side encryption configuration",
		"aws_s3_bucket.y[0] has no server-side encryption configuration",
	} with input as plan(
		[
			instance(resource("aws_s3_bucket", "x", {}), 0),
			instance(resource("aws_s3_bucket", "x", {}), 1),
			instance(resource(sse, "x", {"rule": sse_rule}), 0),
			instance(resource("aws_s3_bucket", "y", {}), 0),
			instance(resource("aws_s3_bucket", "y", {}), 1),
			instance(resource(sse, "y", {"rule": sse_rule}), 0),
			instance(resource("aws_s3_bucket", "f", {}), "a"),
			instance(resource("aws_s3_bucket", "f", {}), "b"),
			instance(resource(sse, "f", {"rule": sse_rule}), "a"),
			resource(sse, "fa", {"rule": sse_rule}),
		],
		[],
		[
			sse_config("x", {"references": ["aws_s3_bucket.x", "count.index"]}),
			sse_config("y", {"references": ["aws_s3_bucket.y[1].id", "aws_s3_bucket.y[1]", "aws_s3_bucket.y"]}),
			sse_config("f", {"references": ["aws_s3_bucket.f", "each.key"]}),
			sse_config("fa", {"references": ["aws_s3_bucket.f[\"b\"].id", "aws_s3_bucket.f[\"b\"]", "aws_s3_bucket.f"]}),
		],
	)
}

# `count = 0`: configuration still lists the config, planned_values does not.
test_sse_config_with_no_instances_denies if {
	encryption.deny == {"aws_s3_bucket.z has no server-side encryption configuration"} with input as plan(
		[resource("aws_s3_bucket", "z", {"bucket": "logs-z"})],
		[],
		[object.union(sse_config("z", {"references": ["aws_s3_bucket.z.id", "aws_s3_bucket.z"]}), {"count_expression": {"constant_value": 0}})],
	)
}

test_inline_sse_block_passes if {
	count(encryption.deny) == 0 with input as plan(
		[resource("aws_s3_bucket", "inline", {"bucket": "inline", "server_side_encryption_configuration": [{"rule": sse_rule}]})],
		[],
		[],
	)
}

# An unset storage_encrypted plans as null.
test_unencrypted_db_denies if {
	encryption.deny == {"aws_db_instance.plain has storage_encrypted = false; AWS RDS at-rest encryption is required"} with input as plan(
		[
			resource("aws_db_instance", "plain", {"engine": "postgres", "storage_encrypted": null}),
			resource("aws_db_instance", "enc", {"engine": "postgres", "storage_encrypted": true}),
		],
		[],
		[],
	)
}

test_aws_managed_secret_kms_alias_denies if {
	encryption.deny == {"aws_secretsmanager_secret.managed uses an AWS-managed KMS alias (alias/aws/secretsmanager); customer-managed key required for compliance scenarios"} with input as plan(
		[
			resource("aws_secretsmanager_secret", "managed", {"name": "managed", "kms_key_id": "alias/aws/secretsmanager"}),
			resource("aws_secretsmanager_secret", "customer", {"name": "customer", "kms_key_id": "alias/app"}),
		],
		[],
		[],
	)
}
