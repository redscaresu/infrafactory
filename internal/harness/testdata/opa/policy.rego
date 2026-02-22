package test.plan

import rego.v1

deny contains "planned_values.root_module is required" if {
	not input.planned_values.root_module
}
