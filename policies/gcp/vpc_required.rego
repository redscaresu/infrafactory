# vpc_required: deny compute and GKE resources that aren't bound to an explicit VPC/subnetwork.
# Targets: google_compute_instance, google_container_cluster
# google_compute_instance must declare network_interface[0].subnetwork.
# google_container_cluster must declare either network or subnetwork.
package gcp.vpc_required

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "google_compute_instance"
	not has_subnetwork(resource)
	msg := sprintf(
		"%s has no network_interface.subnetwork — must be attached to an explicit VPC subnetwork",
		[resource.address],
	)
}

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "google_container_cluster"
	not has_cluster_network(resource)
	msg := sprintf(
		"%s has no network or subnetwork — GKE clusters must reference an explicit VPC",
		[resource.address],
	)
}

has_subnetwork(resource) if {
	nic := resource.values.network_interface[_]
	nic.subnetwork != null
	nic.subnetwork != ""
}

has_cluster_network(resource) if {
	resource.values.network != null
	resource.values.network != ""
}

has_cluster_network(resource) if {
	resource.values.subnetwork != null
	resource.values.subnetwork != ""
}
