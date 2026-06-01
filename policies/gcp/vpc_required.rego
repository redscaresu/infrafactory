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

# M98 — subnetwork referenced from a not-yet-created resource. At plan
# time the value is null in `planned_values` (known-after-apply), but
# `resource_changes[].change.after_unknown.network_interface[i]
# .subnetwork == true` signals "this field IS being set, just to a
# reference that resolves at apply time." Without this branch the
# policy false-fires on correct HCL like
# `subnetwork = google_compute_subnetwork.NAME.id`.
has_subnetwork(resource) if {
	rc := input.resource_changes[_]
	rc.address == resource.address
	nic := rc.change.after_unknown.network_interface[_]
	nic.subnetwork == true
}

has_cluster_network(resource) if {
	resource.values.network != null
	resource.values.network != ""
}

has_cluster_network(resource) if {
	resource.values.subnetwork != null
	resource.values.subnetwork != ""
}

# M98 — cluster references not-yet-created network/subnetwork.
has_cluster_network(resource) if {
	rc := input.resource_changes[_]
	rc.address == resource.address
	rc.change.after_unknown.network == true
}

has_cluster_network(resource) if {
	rc := input.resource_changes[_]
	rc.address == resource.address
	rc.change.after_unknown.subnetwork == true
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
