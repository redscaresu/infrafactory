# region_restriction: deny resources placed outside an allowlist of AWS regions.
# Targets: any resource with a `region` value or that lives in an inferred
# region. The allowlist is read from `data.region_allowlist`. When that
# data document is undefined, a sane default is used.
#
# Mirrors policies/gcp/region_restriction.rego but scoped to `aws_*`
# resource types. Per fakeaws/concepts.md "Required surface" item 15
# (S43-T11). Mixing GCP and AWS allowlists would deny e.g. AWS regions
# under the GCP allowlist; both packages stay clean by checking the
# resource type prefix first.
package aws.region_restriction

import rego.v1

default allowlist := ["us-east-1", "eu-west-1"]

allowlist := data.region_allowlist if {
	data.region_allowlist
}

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	startswith(resource.type, "aws_")
	region := resource.values.region
	region != null
	region != ""
	not region_allowed(region)
	msg := sprintf(
		"%s is in region %s — must be one of %v",
		[resource.address, region, allowlist],
	)
}

# AWS resources sometimes carry their region in the `availability_zone`
# field (e.g. EBS volumes, EC2 instances when not using VPC defaults).
# A zone like "us-east-1a" is allowed when the allowlist contains
# "us-east-1" — same logic the GCP analog uses for zone strings.
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	startswith(resource.type, "aws_")
	az := resource.values.availability_zone
	az != null
	az != ""
	not zone_allowed(az)
	msg := sprintf(
		"%s is in availability_zone %s — must be in one of %v",
		[resource.address, az, allowlist],
	)
}

# Layer 2 — fakeaws state surface. AWS bucket region lives on
# state.s3.buckets[].region.
deny_state contains msg if {
	bucket := input.s3.buckets[_]
	bucket.region != null
	bucket.region != ""
	not region_allowed(bucket.region)
	msg := sprintf("S3 bucket %s region %s not in allowlist %v", [bucket.name, bucket.region, allowlist])
}

region_allowed(region) if {
	allowlist[_] == region
}

zone_allowed(az) if {
	# Direct match (rare — AZs include a trailing letter).
	allowlist[_] == az
}

zone_allowed(az) if {
	# Trim trailing letter to compare against region.
	region := allowlist[_]
	startswith(az, sprintf("%s", [region]))
}
