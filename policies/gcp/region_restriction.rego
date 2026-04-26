# region_restriction: deny resources placed outside an allowlist of GCP regions/locations.
# Targets: any resource with a `region` or `location` value.
# The allowlist is read from `data.region_allowlist`. When that data document
# is undefined, a sane default of common low-latency, low-carbon regions is used.
package gcp.region_restriction

import rego.v1

default allowlist := ["us-central1", "europe-west1", "europe-west4"]

allowlist := data.region_allowlist if {
	data.region_allowlist
}

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	region := resource.values.region
	region != null
	region != ""
	not region_allowed(region)
	msg := sprintf(
		"%s is in region %s — must be one of %v",
		[resource.address, region, allowlist],
	)
}

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	location := resource.values.location
	location != null
	location != ""
	not location_allowed(location)
	msg := sprintf(
		"%s is in location %s — must be one of %v",
		[resource.address, location, allowlist],
	)
}

region_allowed(region) if {
	allowlist[_] == region
}

# A location may be a region (e.g. "us-central1") or a zone within an
# allowed region (e.g. "us-central1-a"). Accept both shapes.
location_allowed(location) if {
	allowlist[_] == location
}

location_allowed(location) if {
	region := allowlist[_]
	startswith(location, sprintf("%s-", [region]))
}
