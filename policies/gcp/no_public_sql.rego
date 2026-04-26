# no_public_sql: deny Cloud SQL instances exposed to the public internet.
# Targets: google_sql_database_instance
# A Cloud SQL instance is considered public if it has ipv4_enabled = true
# without a private_network attached, or if any authorized_networks entry
# allows traffic from 0.0.0.0/0.
package gcp.no_public_sql

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "google_sql_database_instance"
	settings := resource.values.settings[_]
	ip_cfg := settings.ip_configuration[_]
	ip_cfg.ipv4_enabled == true
	not has_private_network(ip_cfg)
	msg := sprintf(
		"%s has ipv4_enabled without a private_network — public access allowed",
		[resource.address],
	)
}

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "google_sql_database_instance"
	settings := resource.values.settings[_]
	ip_cfg := settings.ip_configuration[_]
	net := ip_cfg.authorized_networks[_]
	net.value == "0.0.0.0/0"
	msg := sprintf(
		"%s authorizes 0.0.0.0/0 — public access allowed",
		[resource.address],
	)
}

has_private_network(ip_cfg) if {
	ip_cfg.private_network != null
	ip_cfg.private_network != ""
}

# Layer 2 — fakegcp state shape uses camelCase. A 0.0.0.0/0 entry in
# authorizedNetworks means the public internet can reach the instance.
deny_state contains msg if {
	sql := input.sql.instances[_]
	auth := sql.settings.ipConfiguration.authorizedNetworks[_]
	auth.value == "0.0.0.0/0"
	msg := sprintf("Cloud SQL instance %s exposes 0.0.0.0/0 in authorizedNetworks", [sql.name])
}

# Layer 2 — ipv4Enabled with no privateNetwork configured implies a
# public IP path, which Cloud SQL surfaces by default.
deny_state contains msg if {
	sql := input.sql.instances[_]
	sql.settings.ipConfiguration.ipv4Enabled == true
	not sql.settings.ipConfiguration.privateNetwork
	msg := sprintf("Cloud SQL instance %s has ipv4Enabled with no privateNetwork", [sql.name])
}
