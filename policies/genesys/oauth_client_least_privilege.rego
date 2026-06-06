# oauth_client_least_privilege: deny genesyscloud_oauth_client
# resources whose declared scopes include `*` or whose
# authorized_grant_type isn't one of the documented values
# (CLIENT_CREDENTIALS | CODE | TOKEN | SAML2BEARER).
#
# Least-privilege enforcement at the static layer. Catches the
# common "scopes = [\"*\"]" foot-gun before it reaches the mock.
package genesys.oauth_client_least_privilege

import rego.v1

# Allowed grant types per Genesys public docs.
allowed_grant_types := {
	"CLIENT_CREDENTIALS",
	"CODE",
	"TOKEN",
	"SAML2BEARER",
}

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "genesyscloud_oauth_client"
	scopes := resource.values.scopes
	scopes != null
	scope := scopes[_]
	scope == "*"
	msg := sprintf(
		"%s declares scope=\"*\" — explicit per-API scopes required",
		[resource.address],
	)
}

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "genesyscloud_oauth_client"
	grant := resource.values.authorized_grant_type
	grant != null
	grant != ""
	not allowed_grant_types[grant]
	msg := sprintf(
		"%s authorized_grant_type=%s — must be one of %v",
		[resource.address, grant, allowed_grant_types],
	)
}
