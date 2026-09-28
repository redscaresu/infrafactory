package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

// The claim ADR-0034 section 4 and every deny_state policy comment make
// (ADR-0028): deny_state reads the Layer 2 mock's state even on a run
// whose Layer 3 apply succeeded. The only state criteria evaluation is
// handed is the mock deploy's snapshot, so an accept-inbound group in
// that snapshot is reported, as a mock_deploy failure.
//
// A pin, not a proof of removal: it holds the claim true for this
// entry point, and fails if deny_state is ever wired to real state
// without the comments changing with it.
func TestDenyStateReadsTheMockSnapshotAfterALayer3Apply(t *testing.T) {
	cfg := config.Default()
	cfg.Validation.Layers.SandboxDeploy.Enabled = true
	cfg.Paths.Policies = filepath.Join("..", "..", "policies")
	cfg.ConstraintPolicies = map[string]string{
		"default_deny_ingress": "scaleway/default_deny_ingress.rego",
	}
	rt := &CommandRuntime{Config: cfg}

	sc := scenario.Scenario{
		Name:  "web-live-paris",
		Cloud: "scaleway",
		AcceptanceCriteria: []scenario.AcceptanceCriterion{
			{Type: "policy", Expect: "pass", Check: "default_deny_ingress"},
		},
	}
	snapshot, err := json.Marshal(map[string]any{
		"instance": map[string]any{"security_groups": []any{map[string]any{
			"id": "sg-mock", "name": "web", "inbound_default_policy": "accept",
		}}},
	})
	require.NoError(t, err)

	const sandboxApplied = true
	_, failures := evaluateSupportedCriteria(context.Background(), sc, rt,
		&harness.MockDeployResult{StateSnapshot: snapshot}, sandboxApplied)

	require.Len(t, failures, 1)
	assert.Equal(t, "mock_deploy", failures[0].Layer)
	assert.Equal(t, "default_deny_ingress", failures[0].Policy)
	assert.Contains(t, failures[0].Detail, "sg-mock")
}

// The same claim for AWS, through the checked-in configuration: its flat
// map sends default_deny_ingress to the Scaleway policy, which reads no
// AWS state, so this fails if cloudConstraintPolicies loses the aws entry.
// The snapshot is fakeaws's captured /mock/state with public SSH on web.
func TestDenyStateReadsTheMockSnapshotAfterALayer3ApplyAWS(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "infrafactory.yaml"))
	require.NoError(t, err)
	cfg.Validation.Layers.SandboxDeploy.Enabled = true
	cfg.Paths.Policies = filepath.Join("..", "..", "policies")
	rt := &CommandRuntime{Config: cfg}

	sc := scenario.Scenario{
		Name:  "aws-web-live",
		Cloud: "aws",
		AcceptanceCriteria: []scenario.AcceptanceCriterion{
			{Type: "policy", Expect: "pass", Check: "default_deny_ingress"},
		},
	}
	snapshot, err := os.ReadFile(filepath.Join("..", "harness", "testdata", "ingress", "aws", "state_deny_ssh_ipv4.json"))
	require.NoError(t, err)
	var state struct {
		EC2 struct {
			SecurityGroups []struct {
				ID        string `json:"id"`
				GroupName string `json:"group_name"`
			} `json:"security_groups"`
		} `json:"ec2"`
	}
	require.NoError(t, json.Unmarshal(snapshot, &state))
	web := ""
	for _, group := range state.EC2.SecurityGroups {
		if group.GroupName == "web" {
			web = group.ID
		}
	}
	require.NotEmpty(t, web)

	const sandboxApplied = true
	_, failures := evaluateSupportedCriteria(context.Background(), sc, rt,
		&harness.MockDeployResult{StateSnapshot: snapshot}, sandboxApplied)

	require.Len(t, failures, 1)
	assert.Equal(t, "mock_deploy", failures[0].Layer)
	assert.Equal(t, "default_deny_ingress", failures[0].Policy)
	assert.Contains(t, failures[0].Detail, web)
}
