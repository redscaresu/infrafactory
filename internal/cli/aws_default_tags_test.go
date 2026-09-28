package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// modelRunIDTF is a model trying to supply the run id itself: its own
// default_tags in a provider block the wiring replaces.
const modelRunIDTF = `provider "aws" {
  region = "us-east-1"
  default_tags {
    tags = {
      "infrafactory-run-id" = "model-chosen"
    }
  }
}
`

const runIDScenarioYAML = `scenario: aws-run-id
version: "1.0"
cloud: aws
description: "infrafactory-run-id: scenario-chosen"
resources:
  networking:
    vpc: true
acceptance_criteria:
  - type: policy
    check: region_restriction
    params:
      region: us-east-1
    expect: pass
`

// The run's id, and only the run's id, reaches default_tags: not the
// model's provider block, not the scenario YAML.
func TestRunWritesItsOwnIDIntoAWSDefaultTags(t *testing.T) {
	h := newCommandTestHarness(t)
	scenarioPath := filepath.Join(h.WorkspaceDir, "scenarios", "training", "aws-run-id.yaml")
	mustWriteFile(t, scenarioPath, runIDScenarioYAML)

	opts := isolatedRunOpts(h, nil)
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{
				"main.tf":      []byte(sqsQueueTF),
				"providers.tf": []byte(modelRunIDTF),
			}}, nil
		}),
		Static:     &fakeStaticHarness{result: &harness.StaticResult{PlanJSON: []byte(`{}`)}},
		MockDeploy: &fakeMockDeployHarness{result: &harness.MockDeployResult{StateSnapshot: []byte(`{}`)}},
		Destroy:    &fakeDestroyHarness{result: &harness.DestroyResult{StateSnapshot: []byte(`{}`)}},
	}
	cmd := newRunCommandForTest(opts)
	cmd.RunE = withRuntimeWithOptions("run", opts, sealedHandler(io.Discard, runRunCommand))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath, "--repair-iterations-max", "1"})

	_ = cmd.ExecuteContext(context.WithValue(context.Background(), runIDContextKey{}, testRunID))

	providers, err := os.ReadFile(filepath.Join(h.RunstoreRoot(), "aws-run-id", testRunID, "generated", "providers.tf"))
	require.NoError(t, err)
	assert.Contains(t, string(providers), `"infrafactory-run-id" = "`+testRunID+`"`)
	assert.NotContains(t, string(providers), "model-chosen")
	assert.NotContains(t, string(providers), "scenario-chosen")
}

// A resource's own tag of the key overrides default_tags, so model HCL
// that names it is refused, and the run feeds the refusal back.
func TestAWSWiringRefusesAModelRunIDTag(t *testing.T) {
	t.Parallel()
	for name, tf := range map[string]string{
		"resource tag": `resource "aws_sqs_queue" "jobs" {
  name = "jobs"
  tags = { infrafactory-run-id = "model-chosen" }
}
`,
		"through a local": `locals {
  tags = { "infrafactory-run-id" = "model-chosen" }
}

resource "aws_sqs_queue" "jobs" {
  name = "jobs"
  tags = local.tags
}
`,
	} {
		files := map[string][]byte{"main.tf": []byte(tf)}
		err := ensureAwsProviderWiring(files, config.Config{}, testRunID)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "main.tf", name)
		assert.Contains(t, err.Error(), `"infrafactory-run-id"`, name)
		assert.NotContains(t, files, "providers.tf", "%s: nothing is written for refused HCL", name)
	}

	commented := map[string][]byte{"main.tf": []byte(`# infrafactory-run-id comes from default_tags.
resource "aws_sqs_queue" "jobs" {
  name = "jobs" // not infrafactory-run-id
}
`)}
	require.NoError(t, ensureAwsProviderWiring(commented, config.Config{}, testRunID), "a comment is not a tag")
}

// Outside a run (a bare `generate`) there is no run id to tag with.
func TestAWSProviderBlockWithoutRunIDHasNoDefaultTags(t *testing.T) {
	t.Parallel()
	assert.NotContains(t, buildAwsProviderBlock("us-east-1", ""), "default_tags")
}
