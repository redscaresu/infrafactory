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
	# Only police GCP resources. The Scaleway side has its own
	# region_restriction.rego; mixing them would deny Scaleway regions
	# like fr-par/nl-ams/pl-waw under the GCP allowlist.
	startswith(resource.type, "google_")
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
	startswith(resource.type, "google_")
	location := resource.values.location
	location != null
	location != ""
	not location_allowed(location)
	msg := sprintf(
		"%s is in location %s — must be one of %v",
		[resource.address, location, allowlist],
	)
}

# zonal GCP resources (google_compute_instance, google_compute_disk,
# google_container_node_pool, …) carry their region in `zone`. The
# allowlist semantics match `location`: a zone is allowed if the zone
# string itself is in the allowlist OR if it's prefixed by an allowed
# region (e.g. `us-central1-a` is allowed when the allowlist contains
# `us-central1`).
deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	startswith(resource.type, "google_")
	zone := resource.values.zone
	zone != null
	zone != ""
	not location_allowed(zone)
	msg := sprintf(
		"%s is in zone %s — must be one of %v",
		[resource.address, zone, allowlist],
	)
}

region_allowed(region) if {
	allowlist[_] == region
}

# Layer 2 — fakegcp state surface. Compute instances surface their
# zone as a full self-link URL (e.g.
# http://HOST/compute/v1/projects/PROJECT/zones/europe-west1-a) since
# fakegcp/handlers/compute.go:CreateInstance rewrites `zone` to the
# self-link before persisting. zone_short strips the trailing path
# segment so location_allowed can match the bare zone form.
zone_short(z) := z if {
	not contains(z, "/")
}

zone_short(z) := s if {
	contains(z, "/")
	parts := split(z, "/")
	s := parts[count(parts) - 1]
}

deny_state contains msg if {
	inst := input.compute.instances[_]
	inst.zone != ""
	z := zone_short(inst.zone)
	not location_allowed(z)
	msg := sprintf("compute instance %s zone %s not in allowlist %v", [inst.name, z, allowlist])
}

deny_state contains msg if {
	sql := input.sql.instances[_]
	sql.region != ""
	not region_allowed(sql.region)
	msg := sprintf("Cloud SQL instance %s region %s not in allowlist %v", [sql.name, sql.region, allowlist])
}

# A location may be a region (e.g. "us-central1") or a zone within an
# allowed region (e.g. "us-central1-a"). Accept both shapes. Cloud
# Storage normalises bucket locations to upper case ("US-CENTRAL1") so
# we lower-case before comparing.
location_allowed(location) if {
	allowlist[_] == lower(location)
}

location_allowed(location) if {
	region := allowlist[_]
	startswith(lower(location), sprintf("%s-", [region]))
}
