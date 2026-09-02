package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A NAME, resolved here, never a path from the caller. `deploy` takes a
// filesystem path, and accepting one over HTTP would let a request name
// any YAML on the machine -- including one outside the scenarios tree
// that the layers have never seen.
func TestResolveScenarioByNameMatchesTheDeclaredNameNotTheFilename(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "training"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "training", "unrelated-filename.yaml"),
		[]byte(`scenario: the-declared-name
version: "1.0"
cloud: scaleway
description: filename and name deliberately differ
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`), 0o644))

	got, err := resolveScenarioByName(root, "the-declared-name")

	require.NoError(t, err)
	assert.Contains(t, got, "unrelated-filename.yaml")
}

func TestResolveScenarioByNameRefusesWhatItCannotFind(t *testing.T) {
	_, err := resolveScenarioByName(t.TempDir(), "not-here")
	require.Error(t, err)

	_, err = resolveScenarioByName(t.TempDir(), "")
	require.Error(t, err, "an empty name must not match the first file it walks past")
}

// A path is not a name. Accepting one would defeat the point of
// resolving by name at all.
func TestResolveScenarioByNameDoesNotAcceptAPath(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "sc.yaml"),
		[]byte(`scenario: real-one
version: "1.0"
cloud: scaleway
description: x
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`), 0o644))

	for _, attempt := range []string{
		filepath.Join(root, "sc.yaml"),
		"../../../etc/passwd",
		"/etc/passwd",
	} {
		_, err := resolveScenarioByName(root, attempt)
		require.Error(t, err, "%q is a path, not a scenario name", attempt)
	}
}

// A file that will not parse must not fail the search for an unrelated
// scenario: one broken YAML in the tree would otherwise make every
// deploy impossible.
func TestResolveScenarioByNameSkipsFilesItCannotParse(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "broken.yaml"), []byte("{{{not yaml"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "good.yaml"),
		[]byte(`scenario: still-findable
version: "1.0"
cloud: scaleway
description: x
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`), 0o644))

	got, err := resolveScenarioByName(root, "still-findable")

	require.NoError(t, err)
	assert.Contains(t, got, "good.yaml")
}

// A deploy whose result cannot be read is a FAILURE, not an empty
// success: the apply may well have created infrastructure, and an empty
// result would say the opposite.
func TestDeployOutcomeTreatsUnreadableOutputAsAFailure(t *testing.T) {
	got := deployOutcome("not json at all", "init...\napply...", nil)

	assert.False(t, got.Clean)
	require.NotEmpty(t, got.Failures)
	assert.Contains(t, got.Failures[0].Detail, "whether infrastructure was created is unknown")
}

func TestDeployOutcomeReadsTheCommandsOwnVerdict(t *testing.T) {
	// The ENVELOPE the command actually writes. An earlier version of
	// this test marshalled a bare OutputResult -- the shape I assumed
	// rather than the one emitted -- and passed while a successful
	// deploy was being reported as a 409 after creating infrastructure.
	payload, err := json.Marshal(MachineOutput{
		Schema: "infrafactory.output.v1",
		Result: OutputResult{
			Command: "deploy", Status: CommandStatusSuccess,
			Stages: []StageSummary{{Layer: "sandbox_deploy", Stage: "apply", Status: StageStatusPass}},
		},
	})
	require.NoError(t, err)

	got := deployOutcome(string(payload), "", nil)

	assert.True(t, got.Clean)
	require.Len(t, got.Steps, 1)
	assert.Equal(t, "apply", got.Steps[0].Stage)
}

// A failed deploy carries its stages: they name the leaked project id
// and how to remove it by hand, which is the one handle an operator has
// on the path where they most need it.
func TestDeployOutcomeCarriesTheFailureDetail(t *testing.T) {
	payload, err := json.Marshal(MachineOutput{
		Schema: "infrafactory.output.v1",
		Result: OutputResult{
			Command: "deploy", Status: CommandStatusFailed,
			Failures: []FailureSummary{{
				Layer: "sandbox_deploy", Stage: "run_project",
				Detail: "project 7c98d82e is live and could not be deleted",
			}},
		},
	})
	require.NoError(t, err)

	got := deployOutcome(string(payload), "", errors.New("deploy failed"))

	assert.False(t, got.Clean)
	require.Len(t, got.Failures, 1)
	assert.Contains(t, got.Failures[0].Detail, "7c98d82e")
}

// The defect that made pass 119 a P1: unmarshalling the envelope's
// CONTENTS into an OutputResult succeeds with every field zero, because
// unknown keys are ignored. A successful deploy then reads as unclean
// with no steps and no failures, and the endpoint answers 409 after
// infrastructure was created.
//
// A parse that cannot fail is worse than one that does: there is nothing
// to notice.
func TestDeployOutcomeRejectsOutputThatIsNotTheCommandsEnvelope(t *testing.T) {
	// Valid JSON, wrong shape -- exactly what a bare OutputResult, or
	// any other object, looks like to a permissive unmarshal.
	got := deployOutcome(`{"command":"deploy","status":"success"}`, "applying...", nil)

	assert.False(t, got.Clean)
	require.NotEmpty(t, got.Failures)
	assert.Contains(t, got.Failures[0].Detail, "could not be read")
}

// And the shape it does understand still works end to end.
func TestDeployOutcomeAcceptsTheRealEnvelope(t *testing.T) {
	payload, err := json.Marshal(MachineOutput{
		Schema: "infrafactory.output.v1",
		Result: OutputResult{Command: "deploy", Status: CommandStatusSuccess},
	})
	require.NoError(t, err)

	assert.True(t, deployOutcome(string(payload), "", nil).Clean)
}

// CommandRuntime.LoadScenario caches: a runtime that loaded one scenario
// refuses a different path with "scenario already loaded from …". Right
// for a CLI process handling one command; wrong for a server, which
// would deploy the first scenario asked for and fail every other one.
func TestDeployerBuildsAFreshRuntimeForEachDeploy(t *testing.T) {
	root := twoScenarios(t)

	built := 0
	deployer := NewLiveDeployer(root, func() (*CommandRuntime, error) {
		built++
		// A runtime that fails fast: this test is about how many are
		// built, not about running a real apply.
		return nil, errors.New("not building a real runtime in a unit test")
	})

	for _, name := range []string{"first-scenario", "second-scenario"} {
		_, err := deployer.Deploy(context.Background(), name, "", nil)
		require.Error(t, err)
	}

	assert.Equal(t, 2, built,
		"a shared runtime would deploy the first scenario and refuse the rest")
}

// A name that resolves to nothing must not consume a runtime build, and
// must say what was wrong.
func TestDeployerRefusesAnUnknownScenarioBeforeBuildingAnything(t *testing.T) {
	built := 0
	deployer := NewLiveDeployer(t.TempDir(), func() (*CommandRuntime, error) {
		built++
		return nil, errors.New("should not be reached")
	})

	_, err := deployer.Deploy(context.Background(), "no-such-scenario", "", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no scenario named")
	assert.Zero(t, built, "resolution comes first")
}

// twoScenarios writes a scenarios tree with two distinct names, which is
// the minimum needed to show the caching problem.
func twoScenarios(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"first-scenario", "second-scenario"} {
		body := []byte("scenario: " + name + `
version: "1.0"
cloud: scaleway
description: x
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`)
		require.NoError(t, os.WriteFile(filepath.Join(root, name+".yaml"), body, 0o644))
	}
	return root
}

// A deploy must not depend on anybody watching: a nil progress writer is
// the ordinary case for a caller that has no hub.
func TestDeployerAcceptsANilProgressWriter(t *testing.T) {
	deployer := NewLiveDeployer(twoScenarios(t), func() (*CommandRuntime, error) {
		return nil, errors.New("not building a real runtime in a unit test")
	})

	_, err := deployer.Deploy(context.Background(), "first-scenario", "", nil)

	// It reaches the runtime factory, which is as far as a unit test
	// takes it. The point is that nil did not panic on the way.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not building a real runtime")
}
