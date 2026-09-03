package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/api"
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

// The nil-progress path, reached directly.
//
// The previous attempt at this asserted through Deploy() with a runtime
// factory that always failed, so it returned before `progress` was ever
// touched: deleting the nil guard entirely still passed, while
// production would panic on the first write for exactly the caller the
// test named.
func TestDeployStderrHandlesANilProgressWriter(t *testing.T) {
	var copy strings.Builder

	w := deployStderr(nil, &copy)

	require.NotNil(t, w)
	_, err := io.WriteString(w, "a line\n")
	require.NoError(t, err, "io.MultiWriter would store the nil writer and panic here")
	assert.Equal(t, "a line\n", copy.String())
}

// And with a writer, both destinations get it: the stream goes out live
// AND is kept, so a failure producing no structured output can still be
// explained.
func TestDeployStderrTeesToBoth(t *testing.T) {
	var live, copy strings.Builder

	_, err := io.WriteString(deployStderr(&live, &copy), "a line\n")

	require.NoError(t, err)
	assert.Equal(t, "a line\n", live.String())
	assert.Equal(t, "a line\n", copy.String())
}

// The lock lives on the server because the page cannot hold it: a
// refresh wipes client state, a second tab never had it, and a `curl`
// never consulted it.
func TestDeployerRefusesASecondDeployOfTheSameScenario(t *testing.T) {
	root := twoScenarios(t)

	// Blocks inside the runtime factory, so the first deploy is
	// genuinely in flight while the second is attempted.
	//
	// Only the FIRST call blocks. Every later one returns at once.
	//
	// If the factory blocked unconditionally, a regression that removes
	// the lock would make the second Deploy reach it and wait for a
	// release that the test has not sent yet -- deadlocking for Go's
	// full timeout instead of failing. A test that hangs on regression
	// is barely better than one that passes, and this test exists to
	// catch exactly that regression.
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	var calls atomic.Int32
	deployer := NewLiveDeployer(root, func() (*CommandRuntime, error) {
		entered <- struct{}{}
		if calls.Add(1) == 1 {
			<-release
		}
		return nil, errors.New("stop here; the lock is what is under test")
	})

	go func() {
		_, _ = deployer.Deploy(context.Background(), "first-scenario", "", nil)
	}()
	<-entered

	assert.Equal(t, []string{"first-scenario"}, deployer.InFlight())

	_, err := deployer.Deploy(context.Background(), "first-scenario", "", nil)
	require.ErrorIs(t, err, api.ErrDeployInProgress,
		"a second deploy of a scenario already in flight must be refused")

	// A DIFFERENT scenario is ordinary and must not be blocked. It
	// returns immediately, so wait for it to appear rather than racing.
	go func() {
		_, _ = deployer.Deploy(context.Background(), "second-scenario", "", nil)
	}()
	<-entered

	close(release)
}

// A scenario stuck marked-as-deploying could never be deployed again
// without restarting the server -- a worse failure than the one the lock
// prevents.
func TestTheLockIsReleasedWhenADeployFails(t *testing.T) {
	deployer := NewLiveDeployer(twoScenarios(t), func() (*CommandRuntime, error) {
		return nil, errors.New("the runtime could not be built")
	})

	_, err := deployer.Deploy(context.Background(), "first-scenario", "", nil)
	require.Error(t, err)

	assert.Empty(t, deployer.InFlight(), "a failed deploy must not hold the name forever")

	// And the scenario is deployable again.
	_, err = deployer.Deploy(context.Background(), "first-scenario", "", nil)
	require.Error(t, err)
	require.NotErrorIs(t, err, api.ErrDeployInProgress)
}

// A name that resolves to nothing must leave no lock behind.
//
// Note what this does NOT pin: mutation testing showed that claiming
// before resolution also passes, because the deferred release fires on
// the resolution failure too. The ordering is tidiness; the RELEASE is
// the thing that matters, and that is what this catches.
func TestAnUnknownScenarioDoesNotHoldTheLock(t *testing.T) {
	deployer := NewLiveDeployer(twoScenarios(t), func() (*CommandRuntime, error) {
		return nil, errors.New("should not be reached")
	})

	_, err := deployer.Deploy(context.Background(), "no-such-scenario", "", nil)
	require.Error(t, err)

	assert.Empty(t, deployer.InFlight())
}
