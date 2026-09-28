package regotest.killed_test

import rego.v1

import data.regotest.killed

# Trimmed from `tofu show -json` of a hashicorp/aws 5.100.0 plan.
public_db := {"planned_values": {"root_module": {"resources": [{
	"address": "aws_db_instance.public",
	"mode": "managed",
	"type": "aws_db_instance",
	"name": "public",
	"provider_name": "registry.opentofu.org/hashicorp/aws",
	"values": {"engine": "postgres", "identifier": "public-db", "publicly_accessible": true},
}]}}}

# Trimmed from fakeaws GET /mock/state after applying an aws_s3_bucket;
# params is what infrafactory adds from the criterion.
bucket_state(region) := {
	"s3": {"buckets": [{
		"arn": "arn:aws:s3:::regotest-logs",
		"created_at": "2026-09-28T05:18:24Z",
		"name": "regotest-logs",
		"region": "us-east-1",
	}]},
	"params": {"region": region},
}

test_public_db_denies if {
	killed.deny == {"aws_db_instance.public is publicly accessible"} with input as public_db
}

test_bucket_outside_region_denies if {
	killed.deny_state == {"S3 bucket regotest-logs is in us-east-1"} with input as bucket_state("eu-west-1")
}
