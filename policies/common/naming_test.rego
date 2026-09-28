package common.naming_test

import rego.v1

import data.common.naming

# planned_values resources, trimmed to what the policy reads.
plan(type, name) := {"planned_values": {"root_module": {"resources": [{
	"address": sprintf("%s.this", [type]),
	"mode": "managed",
	"type": type,
	"values": {"name": name},
}]}}}

denies(type, name) if {
	naming.deny == {sprintf("%s.this has name '%s' - must start with a lowercase letter, use lowercase alphanumeric or hyphens, and not end with a hyphen", [type, name])} with input as plan(type, name)
}

passes(type, name) if {
	count(naming.deny) == 0 with input as plan(type, name)
}

test_malformed_names_deny if {
	denies("scaleway_instance_security_group", "Open_SG")
	denies("scaleway_instance_server", "web-")
	denies("aws_s3_bucket", "1-logs")
}

test_slug_passes if {
	passes("scaleway_instance_security_group", "open-sg")
}

test_null_and_empty_names_pass if {
	passes("scaleway_domain_record", null)
	passes("scaleway_domain_record", "")
}

# A scaleway_instance_private_nic renders no name at all.
test_absent_name_passes if {
	count(naming.deny) == 0 with input as {"planned_values": {"root_module": {"resources": [{
		"address": "scaleway_instance_private_nic.nic",
		"type": "scaleway_instance_private_nic",
		"values": {"tags": null},
	}]}}}
}

test_gcp_resource_path_exempt_only_on_path_named_types if {
	passes("google_secret_manager_secret", "projects/p/secrets/s")
	denies("google_pubsub_topic", "projects/p/topics/t")
}

test_dns_fqdn_exempt_only_on_record_set if {
	passes("google_dns_record_set", "host.example.invalid.")
	denies("google_dns_managed_zone", "example.invalid.")
}

test_aws_dns_name_exempt_only_on_route53 if {
	passes("aws_route53_zone", "example.com")
	passes("aws_route53_record", "www.example.com")
	denies("aws_route53_zone", "Example")
	denies("aws_s3_bucket", "logs.example.com")
}

test_aws_path_name_exempt if {
	passes("aws_secretsmanager_secret", "infrafactory/db/password")
	passes("aws_iam_role", "service-role/Foo_Bar")
	denies("aws_sqs_queue", "infrafactory/queue")
}
