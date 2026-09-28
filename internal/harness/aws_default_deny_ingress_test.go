package harness

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const awsDefaultDenyIngressPolicy = "../../policies/aws/default_deny_ingress.rego"

// awsIngressCaptures holds what testdata/ingress/aws/capture.sh writes:
// hashicorp/aws 5.100.0 plans and fakeaws /mock/state, trimmed only to
// the security-group parts the policy reads.
var awsIngressCaptures = filepath.Join("testdata", "ingress", "aws")

// Each capture names every resource the policy must deny open*, across
// the four ingress shapes, unknowns and module depths, and no other: so
// the denied set is compared whole, a miss and a false deny alike.
func TestAWSDefaultDenyIngressDeniesExactlyTheOpenResourcesInEachCapturedPlan(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"plan.json", "network.json", "network_open.json"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(filepath.Join(awsIngressCaptures, name))
			require.NoError(t, err)
			var plan struct {
				ResourceChanges []struct {
					Address string `json:"address"`
					Name    string `json:"name"`
				} `json:"resource_changes"`
			}
			require.NoError(t, json.Unmarshal(raw, &plan))
			require.NotEmpty(t, plan.ResourceChanges)
			want := []string{}
			for _, rc := range plan.ResourceChanges {
				if strings.HasPrefix(rc.Name, "open") {
					want = append(want, rc.Address)
				}
			}

			failures, err := EvaluatePlanPolicies(t.Context(), raw, []string{awsDefaultDenyIngressPolicy})
			require.NoError(t, err)

			denied := map[string]bool{}
			for _, failure := range failures {
				address, _, _ := strings.Cut(failure.Detail, " ")
				denied[address] = true
			}
			assert.ElementsMatch(t, want, slices.Collect(maps.Keys(denied)))
		})
	}
}

// state_deny_* holds exactly one offending group, web; state_pass_* none.
func TestAWSDefaultDenyIngressJudgesEachCapturedState(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join(awsIngressCaptures, "state_*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(file)
			require.NoError(t, err)

			failures, err := EvaluateStatePolicies(t.Context(), raw, []string{awsDefaultDenyIngressPolicy})
			require.NoError(t, err)

			if !strings.HasPrefix(filepath.Base(file), "state_deny_") {
				assert.Empty(t, failures)
				return
			}
			require.Len(t, failures, 1)
			assert.Contains(t, failures[0].Detail, "deployed security group web (sg-")
		})
	}
}
