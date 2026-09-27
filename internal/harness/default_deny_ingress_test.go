package harness

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const defaultDenyIngressPolicy = "../../policies/scaleway/default_deny_ingress.rego"

// securityGroupPlan renders a plan whose single security group carries
// exactly the given values, so a case can omit inbound_default_policy
// entirely rather than only set it to something wrong.
func securityGroupPlan(t *testing.T, values map[string]any) []byte {
	t.Helper()
	return mockState(t, map[string]any{
		"planned_values": map[string]any{
			"root_module": map[string]any{
				"resources": []any{
					map[string]any{
						"address": "scaleway_instance_security_group.web",
						"type":    "scaleway_instance_security_group",
						"values":  values,
					},
				},
			},
		},
	})
}

// The case that shipped. An omitted inbound_default_policy is rendered
// by the provider as the literal "accept" in planned_values -- measured
// against real Scaleway with provider 2.83.0 on 2026-09-27, not assumed
// -- so Layer 1 can see it without any `configuration` fallback.
func TestDefaultDenyIngressDeniesAnAcceptGroupInThePlan(t *testing.T) {
	t.Parallel()

	plan := securityGroupPlan(t, map[string]any{"inbound_default_policy": "accept"})

	failures, err := EvaluatePlanPolicies(context.Background(), plan, []string{defaultDenyIngressPolicy})
	require.NoError(t, err)

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "scaleway_instance_security_group.web")
	// The denial is repair-loop input: it has to carry the fix, not
	// just the verdict.
	assert.Contains(t, failures[0].Detail, `inbound_default_policy = "drop"`)
}

func TestDefaultDenyIngressAcceptsADropGroupInThePlan(t *testing.T) {
	t.Parallel()

	plan := securityGroupPlan(t, map[string]any{"inbound_default_policy": "drop"})

	failures, err := EvaluatePlanPolicies(context.Background(), plan, []string{defaultDenyIngressPolicy})
	require.NoError(t, err)
	assert.Empty(t, failures)
}

// The undefined-not-false guard. A comparison against a MISSING field is
// undefined in rego, and an undefined rule returns zero results -- which
// reads identically to a rule that found nothing wrong. Written as
// `not inbound_is_drop(...)` so an absent field denies; if a future
// provider stops rendering its own default into the plan, this fails
// closed instead of going quietly green.
func TestDefaultDenyIngressDeniesWhenThePolicyFieldIsAbsentEntirely(t *testing.T) {
	t.Parallel()

	plan := securityGroupPlan(t, map[string]any{"name": "web"})

	failures, err := EvaluatePlanPolicies(context.Background(), plan, []string{defaultDenyIngressPolicy})
	require.NoError(t, err)
	assert.Len(t, failures, 1, "an absent inbound_default_policy must deny, not vacuously pass")
}

// The same property against what the provider actually created. A plan
// rule cannot see a group the API changed after apply.
func TestDefaultDenyIngressChecksDeployedState(t *testing.T) {
	t.Parallel()

	state := mockState(t, map[string]any{
		"instance": map[string]any{
			"security_groups": []any{
				map[string]any{
					"id":                     "sg-1",
					"name":                   "web",
					"inbound_default_policy": "accept",
				},
			},
		},
	})

	failures, err := EvaluateStatePolicies(context.Background(), state, []string{defaultDenyIngressPolicy})
	require.NoError(t, err)

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "sg-1")
	assert.Contains(t, failures[0].Detail, "accept")
}

func TestDefaultDenyIngressAcceptsADropGroupInDeployedState(t *testing.T) {
	t.Parallel()

	state := mockState(t, map[string]any{
		"instance": map[string]any{
			"security_groups": []any{
				map[string]any{
					"id":                     "sg-1",
					"name":                   "web",
					"inbound_default_policy": "drop",
				},
			},
		},
	})

	failures, err := EvaluateStatePolicies(context.Background(), state, []string{defaultDenyIngressPolicy})
	require.NoError(t, err)
	assert.Empty(t, failures)
}

// Scaleway creates a "Default security group" in every project on the
// first instance, with inbound accept, and Terraform never owns it --
// which is why teardown has a purge step. Denying it would fail every
// single run forever, for a shape no repair iteration could fix.
//
// Confirmed against the real API on 2026-09-27: project_default and
// organization_default both true, inbound_default_policy "accept".
func TestDefaultDenyIngressIgnoresTheAPICreatedDefaultGroup(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"project_default", "organization_default"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			state := mockState(t, map[string]any{
				"instance": map[string]any{
					"security_groups": []any{
						map[string]any{
							"id":                     "sg-default",
							"name":                   "Default security group",
							"inbound_default_policy": "accept",
							field:                    true,
						},
					},
				},
			})

			failures, err := EvaluateStatePolicies(context.Background(), state, []string{defaultDenyIngressPolicy})
			require.NoError(t, err)
			assert.Empty(t, failures, "the API-created group is not repairable by generated HCL")
		})
	}
}

// The stated limit, asserted so it cannot drift into an assumed
// guarantee: this policy checks that a DECLARED group is a real
// firewall, not that a server has one. A configuration declaring no
// security group at all leaves its server on the API default and
// passes. The holdout is what catches that.
func TestDefaultDenyIngressDoesNotRequireAServerToHaveAGroup(t *testing.T) {
	t.Parallel()

	state := mockState(t, map[string]any{
		"instance": map[string]any{
			"servers":         []any{map[string]any{"id": "srv-1", "name": "web"}},
			"security_groups": []any{},
		},
	})

	failures, err := EvaluateStatePolicies(context.Background(), state, []string{defaultDenyIngressPolicy})
	require.NoError(t, err)
	assert.Empty(t, failures)
}
