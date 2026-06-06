# queue_must_have_wrapup: deny genesyscloud_routing_queue resources
# that don't reference at least one wrap-up code. Real Genesys
# operations require agents to select a wrap-up code at the end of
# every interaction; queues without a wrap-up reference are an
# operational foot-gun even though the API accepts them.
#
# Layer 2 placeholder: mocks don't enforce this today (fakegenesys
# accepts queues with no wrap-up codes per Reverse Fidelity since the
# Genesys spec doesn't pin it). Static layer enforces.
package genesys.queue_must_have_wrapup

import rego.v1

deny contains msg if {
	resource := input.planned_values.root_module.resources[_]
	resource.type == "genesyscloud_routing_queue"
	wrapups := resource.values.wrapup_codes
	count(wrapups) == 0
	msg := sprintf(
		"%s has no wrapup_codes — every queue must reference at least one wrapup code",
		[resource.address],
	)
}
