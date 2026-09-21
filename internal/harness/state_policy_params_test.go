package harness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockState(t *testing.T, state map[string]any) []byte {
	t.Helper()
	payload, err := json.Marshal(state)
	require.NoError(t, err)
	return payload
}

// The same question the plan rule asks, of what the provider actually
// created. A provider can accept `region = fr-par` and create
// somewhere else; only the state reading would notice.
func TestRegionRestrictionChecksDeployedState(t *testing.T) {
	state := mockState(t, map[string]any{
		"instance": map[string]any{
			"servers": []any{map[string]any{"name": "web", "zone": "fr-par-1"}},
		},
		"vpc": map[string]any{
			"vpcs": []any{map[string]any{"name": "stray-vpc", "region": "nl-ams"}},
		},
	})

	failures, err := EvaluateStatePoliciesWithInput(context.Background(), state,
		map[string]any{"params": map[string]any{"region": "fr-par"}},
		[]string{"../../policies/scaleway/region_restriction.rego"})
	require.NoError(t, err)

	require.Len(t, failures, 1, "the fr-par-1 server satisfies fr-par; the nl-ams vpc does not")
	assert.Contains(t, failures[0].Detail, "stray-vpc")
	assert.Contains(t, failures[0].Detail, "nl-ams")
}

// A zone satisfies its region: fr-par-1 is in fr-par.
func TestRegionRestrictionAcceptsAZoneInsideTheRegion(t *testing.T) {
	state := mockState(t, map[string]any{
		"lb": map[string]any{
			"lbs": []any{map[string]any{"name": "lb", "zone": "fr-par-1"}},
			"ips": []any{map[string]any{"id": "ip-1", "region": "fr-par", "zone": "fr-par-2"}},
		},
	})

	failures, err := EvaluateStatePoliciesWithInput(context.Background(), state,
		map[string]any{"params": map[string]any{"region": "fr-par"}},
		[]string{"../../policies/scaleway/region_restriction.rego"})
	require.NoError(t, err)
	assert.Empty(t, failures)
}

// Walked generically rather than by listing known collections: an
// explicit list silently stops covering each new resource type, and a
// region rule that skips the resource you just added is the failure
// this policy exists to prevent.
func TestRegionRestrictionCoversResourceTypesItWasNeverToldAbout(t *testing.T) {
	state := mockState(t, map[string]any{
		"somethingnew": map[string]any{
			"widgets": []any{map[string]any{"name": "widget-1", "zone": "pl-waw-1"}},
		},
	})

	failures, err := EvaluateStatePoliciesWithInput(context.Background(), state,
		map[string]any{"params": map[string]any{"region": "fr-par"}},
		[]string{"../../policies/scaleway/region_restriction.rego"})
	require.NoError(t, err)

	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "widget-1")
}

// Without params the rule cannot fire, so it MUST NOT be reachable
// with them missing -- that was the state of the world before the
// evaluator forwarded them, and it reported a clean pass.
func TestRegionRestrictionFindsNothingWithoutParams(t *testing.T) {
	state := mockState(t, map[string]any{
		"vpc": map[string]any{"vpcs": []any{map[string]any{"name": "v", "region": "nl-ams"}}},
	})

	failures, err := EvaluateStatePoliciesWithInput(context.Background(), state, nil,
		[]string{"../../policies/scaleway/region_restriction.rego"})
	require.NoError(t, err)
	assert.Empty(t, failures,
		"no params means no comparison -- which is exactly why the caller must report the gap rather than trust this")
}

// Rego rules are UNDEFINED rather than false when absent, and an
// undefined rule returns zero results -- identical to a defined rule
// that found nothing. Asking the AST is the only way to tell them
// apart.
func TestPolicyFileDefinesRuleDistinguishesAbsentFromPassing(t *testing.T) {
	hasState, err := PolicyFileDefinesRule("../../policies/scaleway/no_public_database.rego", "deny_state")
	require.NoError(t, err)
	assert.True(t, hasState, "no_public_database has always had a state rule")

	hasState, err = PolicyFileDefinesRule("../../policies/scaleway/vpc_required.rego", "deny_state")
	require.NoError(t, err)
	assert.False(t, hasState, "vpc_required is plan-only")

	hasPlan, err := PolicyFileDefinesRule("../../policies/scaleway/vpc_required.rego", "deny")
	require.NoError(t, err)
	assert.True(t, hasPlan)

	// The one this slice added.
	hasState, err = PolicyFileDefinesRule("../../policies/scaleway/region_restriction.rego", "deny_state")
	require.NoError(t, err)
	assert.True(t, hasState, "region_restriction now checks deployed state too")
}

// Answered from the AST, so a rule named in a comment or a string does
// not count as defined -- which a grep would get wrong.
func TestPolicyFileDefinesRuleIgnoresCommentsAndStrings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "decoy.rego")
	require.NoError(t, os.WriteFile(path, []byte(`package scaleway.decoy

import rego.v1

# this file mentions deny_state in a comment
deny contains msg if {
	msg := "deny_state appears in this string too"
}
`), 0o600))

	defined, err := PolicyFileDefinesRule(path, "deny_state")
	require.NoError(t, err)
	assert.False(t, defined, "a mention is not a definition")
}

// A path that is not rego is a caller mistake, and silently returning
// false would make it look like a plan-only policy.
func TestPolicyFileDefinesRuleRefusesANonRegoPath(t *testing.T) {
	_, err := PolicyFileDefinesRule("../../infrafactory.yaml", "deny_state")
	require.Error(t, err)
}

// A stricter `zone` param is an exact match, mirroring the plan rule.
// Without it a criterion asking for fr-par-1 is satisfied by a
// resource in fr-par-2, because the region rule only checks the
// `fr-par` prefix -- making the deployed-state check weaker than the
// plan check it is meant to confirm.
func TestRegionRestrictionEnforcesAStricterZone(t *testing.T) {
	state := mockState(t, map[string]any{
		"instance": map[string]any{
			"servers": []any{map[string]any{"name": "wrong-zone", "zone": "fr-par-2"}},
		},
	})

	failures, err := EvaluateStatePoliciesWithInput(context.Background(), state,
		map[string]any{"params": map[string]any{"region": "fr-par", "zone": "fr-par-1"}},
		[]string{"../../policies/scaleway/region_restriction.rego"})
	require.NoError(t, err)

	require.Len(t, failures, 1, "fr-par-2 satisfies the region and violates the zone")
	assert.Contains(t, failures[0].Detail, "wrong-zone")
	assert.Contains(t, failures[0].Detail, "fr-par-1")
}

// ...and the matching zone passes, so the rule is not simply always red.
func TestRegionRestrictionAcceptsTheExactZone(t *testing.T) {
	state := mockState(t, map[string]any{
		"instance": map[string]any{
			"servers": []any{map[string]any{"name": "right-zone", "zone": "fr-par-1"}},
		},
	})

	failures, err := EvaluateStatePoliciesWithInput(context.Background(), state,
		map[string]any{"params": map[string]any{"region": "fr-par", "zone": "fr-par-1"}},
		[]string{"../../policies/scaleway/region_restriction.rego"})
	require.NoError(t, err)
	assert.Empty(t, failures)
}
