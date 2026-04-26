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

# Layer 2 — fakegcp state. An instance with no networkInterfaces or no
# subnetwork on its primary interface is unscoped. Clusters lacking
# both network and subnetwork are also unscoped.
deny_state contains msg if {
	inst := input.compute.instances[_]
	count(inst.networkInterfaces) == 0
	msg := sprintf("compute instance %s has no networkInterfaces — VPC scoping required", [inst.name])
}

deny_state contains msg if {
	cluster := input.container.clusters[_]
	not cluster.network
	not cluster.subnetwork
	msg := sprintf("GKE cluster %s has no network or subnetwork — VPC scoping required", [cluster.name])
}
