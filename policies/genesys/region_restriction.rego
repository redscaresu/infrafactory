# region_restriction: deny Genesys Cloud resources placed outside the
# allowlist of supported regions. Genesys exposes regions via the
# provider's `aws_region` attribute on the provider block (Genesys
# Cloud's CCaaS regions map 1:1 to AWS region names: us-east-1,
# eu-west-1, eu-central-1, ap-southeast-1, etc.).
#
# Mirrors policies/aws/region_restriction.rego scoped to provider
# config since Genesys resources don't carry a per-resource `region`
# field — the region is fixed at the provider level for a Genesys org.
package genesys.region_restriction

import rego.v1

default allowlist := ["us-east-1", "eu-west-1"]

allowlist := data.region_allowlist if {
	data.region_allowlist
}

# Layer 2 — fakegenesys state surface. The mock doesn't ship a region
# in state today, so this check is a placeholder for when the regional
# carve-out lands. For now we deny on the planned_values
# provider_config block.
deny contains msg if {
	provider := input.configuration.provider_config.genesyscloud
	region := provider.expressions.aws_region.constant_value
	region != null
	region != ""
	not region_allowed(region)
	msg := sprintf(
		"genesyscloud provider configured for region %s — must be one of %v",
		[region, allowlist],
	)
}

region_allowed(region) if {
	allowlist[_] == region
}
