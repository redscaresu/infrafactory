package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunCommandLearnsAGatePitfallFromARefusal covers the wiring, which is
// the half a unit test of the extractor cannot reach: that the run loop
// consults ExtractGatePitfall at all.
//
// The failure text is verbatim from run 20260910T104418Z, which proposed a
// refused instance type in iterations 1, 3 and 5 -- told the permitted set
// every time -- and finished with nothing durable recorded. Two of the five
// iterations in that run were real applies against real Scaleway.
func TestRunCommandLearnsAGatePitfallFromARefusal(t *testing.T) {
	h := newCommandTestHarness(t)

	pitfallsDir := filepath.Join(h.WorkspaceDir, "pitfalls")
	require.NoError(t, os.MkdirAll(pitfallsDir, 0o755))

	refusal := `layer 3 refuses this configuration: compute.tf: scaleway_instance_server web sets type to "PLAY2-NANO"; the gate permits only [DEV1-S DEV1-M]`
	stageErr := &harness.StageError{
		StageResult: harness.StageResult{Stage: "validate", Cmd: []string{"tofu", "validate"}, Stderr: refusal},
		Err:         errors.New(refusal),
	}

	opts := isolatedRunOpts(h, func(cfg config.Config) config.Config {
		cfg.Agent.RepairIterationsMax = 3
		cfg.Paths.Pitfalls = pitfallsDir
		return cfg
	})
	opts.deps = RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte("terraform {}\n")}}, nil
		}),
		Static:     &alternatingStaticHarness{errs: [2]error{stageErr, stageErr}},
		MockDeploy: &fakeMockDeployHarness{},
		Destroy:    &fakeDestroyHarness{},
	}

	cmd := newRunCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{h.ScenarioPath, "--config", h.ConfigPath})
	require.Error(t, cmd.Execute(), "a run that never satisfies the gate must fail")

	data, err := os.ReadFile(filepath.Join(pitfallsDir, "scaleway.yaml"))
	require.NoError(t, err, "the refusal should have been recorded as a pitfall")
	contents := string(data)

	assert.Contains(t, contents, "resource: scaleway_instance_server")
	assert.Contains(t, contents, "the gate permits only")
	// `fix`, not `descriptive`. A refusal names the permitted values, so it
	// is a remedy; tagging it as a symptom is the mislabelling that made
	// the corpus useless to the generator in the first place.
	assert.Contains(t, contents, "source: fix")
	assert.NotContains(t, contents, "compute.tf",
		"the filename is where it was found, not what to do about it")
	_ = strings.TrimSpace
}
