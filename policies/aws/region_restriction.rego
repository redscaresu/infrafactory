# region_restriction: deny AWS placements outside the criterion's
# params.region. Every rule needs params.region, because this policy runs
# on every AWS plan: with no region asked for, nothing is compared.
package aws.region_restriction

import rego.v1

# Plan: the region is the provider's. On hashicorp/aws 5.100.0 no
# generated resource type carries a plan-time region (aws_s3_bucket's is
# computed-only), so each provider_config entry named aws is checked.
#
# A region the plan cannot read denies rather than being resolved: the
# model's own provider block survives when no fakeaws URL is configured
# (ensureAwsProviderWiring), and a var.X or an unset region may come from
# AWS_REGION, which the plan never sees.
deny contains msg if {
	allowed := input.params.region
	some key, provider in input.configuration.provider_config
	provider.name == "aws"
	region := literal_region(provider)
	region != allowed
	msg := sprintf("provider %s is in region %s — must be in %s", [key, region, allowed])
}

deny contains msg if {
	allowed := input.params.region
	some key, provider in input.configuration.provider_config
	provider.name == "aws"
	not literal_region(provider)
	msg := sprintf(
		"provider %s sets no literal region, so the plan cannot tell where it deploys — set region = \"%s\"",
		[key, allowed],
	)
}

deny contains msg if {
	allowed := input.params.region
	resource := input.planned_values.root_module.resources[_]
	startswith(resource.type, "aws_")
	not aws_provider_configured
	msg := sprintf(
		"aws resources are planned with no aws provider configuration, so their region cannot be read — declare provider \"aws\" with region = \"%s\"",
		[allowed],
	)
}

# A zone is in its region: us-east-1a is in us-east-1.
deny contains msg if {
	allowed := input.params.region
	resource := input.planned_values.root_module.resources[_]
	startswith(resource.type, "aws_")
	az := resource.values.availability_zone
	is_string(az)
	az != ""
	not startswith(az, allowed)
	msg := sprintf("%s is in availability_zone %s — must be in %s", [resource.address, az, allowed])
}

# A helper, not an inline is_string: `not is_string(x)` is undefined, not
# true, when x is.
literal_region(provider) := region if {
	region := provider.expressions.region.constant_value
	is_string(region)
}

aws_provider_configured if {
	some provider in input.configuration.provider_config
	provider.name == "aws"
}

# Layer 2: the same question, asked of the Layer 2 mock's state (fakeaws),
# never a cloud's (ADR-0034 section 4).
#
# Only availability_zone there reflects the model. The region fakeaws
# records is the one in the endpoint path (/ec2/region/<region>, read by
# fakeaws handlers/sqs.go and ec2.go), and infrafactory injects that path
# with us-east-1 (buildAwsProviderBlock), so a state region is the harness's
# choice, not the model's. It is still checked: a region that disagrees
# with params.region is a wiring defect worth failing on.

# state_resource is every object in every collection of the mock's state.
# Walked generically, not by listing collections: a list silently stops
# covering each new resource type. The envelope keys are inputs, not
# resources.
state_resource contains resource if {
	some group, collection
	not group in {"params", "state", "target"}
	items := input[group][collection]
	is_array(items)
	resource := items[_]
	is_object(resource)
}

deny_state contains msg if {
	allowed := input.params.region
	resource := state_resource[_]
	region := resource.region
	is_string(region)
	region != ""
	region != allowed
	msg := sprintf(
		"%s is in region %s in the deployed state — must be in %s",
		[state_resource_label(resource), region, allowed],
	)
}

deny_state contains msg if {
	allowed := input.params.region
	resource := state_resource[_]
	az := resource.availability_zone
	is_string(az)
	az != ""
	not startswith(az, allowed)
	msg := sprintf(
		"%s is in availability_zone %s in the deployed state — must be in %s",
		[state_resource_label(resource), az, allowed],
	)
}

state_resource_label(resource) := object.get(
	resource,
	"name",
	object.get(resource, "id", "resource"),
)
