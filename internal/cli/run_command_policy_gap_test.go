package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toFeedbackFailures is the only bridge from the run's failures to the
// learning paths. A field it drops is invisible to every one of them:
// without Policy, DetectPolicyConflict (ADR-0017) never sees a policy name
// and never records a policy gap.
func TestToFeedbackFailuresPreservesEveryFailureSummaryField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   FailureSummary
		want feedback.Failure
	}{
		{
			name: "state policy failure",
			in: FailureSummary{
				Layer: "mock_deploy", Stage: "state_policy", Check: "policy", Policy: "vpc_required",
				Command: "state policy evaluator", Resource: "scaleway_instance_server.web", Detail: "not attached",
			},
			want: feedback.Failure{
				Layer: "mock_deploy", Stage: "state_policy", Check: "policy", Policy: "vpc_required",
				Command: "state policy evaluator", Resource: "scaleway_instance_server.web", Detail: "not attached",
			},
		},
		{
			name: "run stage failure without a policy",
			in:   FailureSummary{Layer: "run", Stage: "iteration_1_test", Check: "test", Command: "infrafactory test", Detail: "exit status 1"},
			want: feedback.Failure{Layer: "run", Stage: "iteration_1_test", Check: "test", Command: "infrafactory test", Detail: "exit status 1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, []feedback.Failure{tt.want}, toFeedbackFailures([]FailureSummary{tt.in}))
		})
	}
}

// TestRunCommandRecordsPolicyGapWhenHCLAlreadyFollowsThePitfall drives the
// stuck path end to end: the generated HCL already does what the
// same-resource pitfall prescribes, and the policy still denies it. That is
// a policy bug, so it belongs in docs/policy-gaps.md, named by the policy.
//
// The deny message deliberately carries no `policy=X.Y` prefix, as real rego
// messages do not: the name can only arrive through the structured field.
func TestRunCommandRecordsPolicyGapWhenHCLAlreadyFollowsThePitfall(t *testing.T) {
	h := newCommandTestHarness(t)
	docsDir := filepath.Join(h.WorkspaceDir, "docs")
	pitfallsDir := h.PitfallsDir()

	mustWriteFile(t, filepath.Join(pitfallsDir, "scaleway.yaml"), `provider: scaleway
pitfalls:
    - resource: scaleway_instance_server
      rule: Attach each `+"`scaleway_instance_server`"+` with a `+"`scaleway_instance_private_nic`"+`.
      source: fix
      discovered_from: fixture
`)
	policyPath := filepath.Join(h.WorkspaceDir, "policies", "vpc_required.rego")
	mustWriteFile(t, policyPath, `package fixture.vpc_required

import rego.v1

deny_state contains "scaleway_instance_server.web[0] is not attached to a private network" if {
	input.instance
}
`)
	scenarioPath := filepath.Join(h.WorkspaceDir, "scenarios", "training", "policy-gap.yaml")
	mustWriteFile(t, scenarioPath, `scenario: policy-gap
version: "1.0"
cloud: scaleway
description: policy gap fixture
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: policy
    check: vpc_required
    expect: pass
`)

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 3
		cfg.Validation.Layers.Destruction.Enabled = false
		cfg.Paths.Docs = docsDir
		cfg.ConstraintPolicies = map[string]string{"vpc_required": policyPath}
		return cfg
	})
	hcl := `resource "scaleway_instance_server" "web" {}
resource "scaleway_instance_private_nic" "web" {}
`
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte(hcl)}}, nil
		}),
		Static: &fakeStaticHarness{result: &harness.StaticResult{
			Stages:   []harness.StageResult{{Stage: "init"}, {Stage: "validate"}, {Stage: "plan"}, {Stage: "show"}},
			PlanJSON: []byte(`{"planned_values":{"root_module":{}}}`),
		}},
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{
			Apply:         harness.StageResult{Stage: "apply"},
			StateSnapshot: []byte(`{"instance": {"servers": []}}`),
		}},
		Destroy: &fakeDestroyHarness{},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})
	require.Error(t, cmd.Execute(), "a policy that always denies must fail the run")
	require.Contains(t, stdout.String(), "check=stuck", "the fixture must reach the stuck path")

	data, err := os.ReadFile(filepath.Join(docsDir, "policy-gaps.md"))
	require.NoError(t, err, "the policy conflict should have been recorded")
	assert.Contains(t, string(data), "| `vpc_required` | `scaleway_instance_server` |")
}
