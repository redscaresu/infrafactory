package common.naming

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	name := resource.values.name
	name != null
	# Empty string is valid for DNS apex records (Route53, Scaleway
	# Domain Record, etc.) — an empty `name` means the record applies
	# at the zone root. The lowercase-slug regex rejects empty strings
	# as malformed; skip them entirely and let the provider validate.
	name != ""
	# Skip names that are clearly server-assigned or protocol-defined
	# values rather than user-given slug identifiers:
	#   - GCP fully-qualified resource paths begin with `projects/`
	#     (e.g. `projects/p/secrets/s` for google_secret_manager_secret).
	#   - DNS record FQDNs end with a trailing dot, but only on
	#     google_dns_record_set (which models its name as an FQDN).
	#     google_dns_managed_zone.name is a slug and a trailing dot
	#     there is a real misconfiguration, so the type guard is
	#     deliberately narrow.
	not is_gcp_resource_path(resource, name)
	not is_dns_fqdn(resource, name)
	not is_aws_dns_name(resource, name)
	not is_aws_path_name(resource, name)
	not regex.match(`^[a-z](?:[a-z0-9-]*[a-z0-9])?$`, name)
	msg := sprintf(
		"%s has name '%s' - must start with a lowercase letter, use lowercase alphanumeric or hyphens, and not end with a hyphen",
		[resource.address, name],
	)
}

is_gcp_resource_path(resource, name) if {
	# Only resource types whose `name` attribute is server-assigned
	# to a fully-qualified GCP resource path skip the slug rule. For
	# everything else — google_pubsub_topic, google_storage_bucket,
	# google_dns_managed_zone, etc. — `name` is a user-controlled
	# slug and a leading "projects/" is an actual misconfiguration.
	gcp_path_named_types[resource.type]
	startswith(name, "projects/")
}

# gcp_path_named_types is the set of google_* resources whose `name`
# in tofu state is the server-assigned fully-qualified path. The
# Terraform google provider exposes the user-controlled leaf via a
# different attribute (secret_id, account_id, etc.) — `name` is for
# reading back, not for setting.
gcp_path_named_types := {
	"google_secret_manager_secret",
	"google_secret_manager_secret_version",
	"google_service_account",
	"google_service_account_key",
}

is_dns_fqdn(resource, name) if {
	# Only google_dns_record_set actually models its `name` as a DNS
	# FQDN (e.g. "host.example.invalid."). google_dns_managed_zone.name
	# is a regular slug like "test-zone" and must still pass the
	# lowercase-alphanumeric check, so we don't exempt the whole
	# `google_dns_*` family.
	resource.type == "google_dns_record_set"
	endswith(name, ".")
}

is_aws_dns_name(resource, name) if {
	# AWS Route53 resources model their `name` as a DNS domain or FQDN
	# (e.g. "example.com" for zones, "www.example.com" for records).
	# Dots are intrinsic to the value, so the lowercase-slug regex is
	# always going to reject them — and that's wrong. The names still
	# need to be DNS-valid; rather than enforce that here, defer to
	# the provider, which validates DNS shape on plan/apply.
	aws_dns_named_types[resource.type]
	contains(name, ".")
}

# aws_dns_named_types is the set of aws_* resources whose `name`
# attribute is a DNS domain or FQDN. The slug rule must not apply.
aws_dns_named_types := {
	"aws_route53_zone",
	"aws_route53_record",
}

is_aws_path_name(resource, name) if {
	# AWS Secrets Manager, IAM resources etc model `name` as a path-
	# like identifier (e.g. "infrafactory/db/password",
	# "service-role/foo"). Slashes + dots + underscores are legal AWS
	# naming chars on these resource types; the lowercase-slug regex
	# rejects them as if they were typos. The provider validates the
	# allowed character set at plan/apply time.
	aws_path_named_types[resource.type]
}

# aws_path_named_types is the set of aws_* resources whose `name`
# attribute is a slash/dot/underscore-tolerant path. Exempt from the
# slug rule entirely.
aws_path_named_types := {
	"aws_secretsmanager_secret",
	"aws_ssm_parameter",
	"aws_iam_role",
	"aws_iam_policy",
	"aws_iam_user",
	"aws_iam_group",
	"aws_iam_instance_profile",
}
