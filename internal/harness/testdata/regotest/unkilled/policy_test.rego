package regotest.unkilled_test

import rego.v1

import data.regotest.unkilled

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
bucket_state := {
	"s3": {"buckets": [{
		"arn": "arn:aws:s3:::regotest-logs",
		"created_at": "2026-09-28T05:18:24Z",
		"name": "regotest-logs",
		"region": "us-east-1",
	}]},
	"params": {"region": "us-east-1"},
}

test_public_db_denies if {
	unkilled.deny == {"aws_db_instance.public is publicly accessible"} with input as public_db
}

# Passes with or without the deny_state body. Were the mutant made by
# deleting the rule, deny_state would be undefined and this test would
# "kill" it; the harness falsifies the body instead, so it does not.
test_bucket_in_region_passes if {
	count(unkilled.deny_state) == 0 with input as bucket_state
}
