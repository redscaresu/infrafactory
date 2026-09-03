package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/redscaresu/infrafactory/internal/api"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

// LiveActions performs teardown and reap for callers that are not a
// cobra command -- today, the UI server (S159b).
//
// It exists so those callers reach the SAME code the CLI does rather
// than a second implementation. Teardown is the guard between an
// automated action and somebody's real infrastructure: it refuses unless
// a run-owned marker and the API's own provenance BOTH say the project
// is infrafactory's, and it declines to mark a deployment released when
// its state has vanished, because doing so would retire the only record
// saying the resources may still be running.
//
// None of that is re-expressed here. `tearDownDeployment` already takes
// a context, a runtime and a store, so this type is a translation layer
// and nothing else -- which is the point. A second implementation of a
// guard is a second thing that can be wrong.
type LiveActions struct {
	runtime *CommandRuntime
}

// NewLiveActions builds the actor from an already-constructed runtime.
func NewLiveActions(runtime *CommandRuntime) *LiveActions {
	return &LiveActions{runtime: runtime}
}

func (l *LiveActions) store() *livestore.FilesystemStore {
	return livestore.NewFilesystemStore(l.runtime.LiveStoreRoot())
}

// Teardown destroys one deployment, by id.
func (l *LiveActions) Teardown(ctx context.Context, id string) (api.ActionResult, error) {
	store := l.store()

	d, err := store.Get(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.ActionResult{}, err
		}
		// An undecodable record still reaches teardown, which reports it
		// as unreclaimable and names `live forget`. The CLI does the
		// same; refusing here would hide a record that may describe
		// running infrastructure.
		d = livestore.Deployment{ID: id, Undecodable: true}
	}

	stages, failures := tearDownDeployment(ctx, l.runtime, store, d)
	return actionResult(stages, failures), nil
}

// Reap destroys every deployment whose TTL has run out.
//
// Note it walks `Reapable` itself rather than calling the cobra command:
// the command's job is flags, progress on stderr and an exit code, none
// of which mean anything here. The per-deployment work is the same
// function.
func (l *LiveActions) Reap(ctx context.Context) (api.ActionResult, error) {
	store := l.store()

	expired, unreadable, err := store.Reapable(time.Now())
	if err != nil {
		return api.ActionResult{}, err
	}

	var stages []StageSummary
	var failures []FailureSummary

	// Surfaced BEFORE any teardown, as the CLI does: an unreadable
	// record may describe running infrastructure this call is about to
	// report as handled.
	for _, readErr := range unreadable {
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "scan", Check: "readable", Command: "live reap",
			Detail: readErr.Error() + " — this record may describe running infrastructure that reap cannot reach",
		})
	}

	if len(expired) == 0 {
		stages = append(stages, StageSummary{
			Layer: "live", Stage: "reap", Status: StageStatusPass,
			Detail: "no expired deployments",
		})
	}

	for _, d := range expired {
		s, f := tearDownDeployment(ctx, l.runtime, store, d)
		stages = append(stages, s...)
		failures = append(failures, f...)
	}

	return actionResult(stages, failures), nil
}

// actionResult translates the CLI's staged output into the neutral shape
// the API seam speaks.
//
// `Clean` is the absence of failures and is reported as its own field,
// because "it worked" and "nothing complained" are different claims and
// ADR-0024 turns on the difference.
func actionResult(stages []StageSummary, failures []FailureSummary) api.ActionResult {
	out := api.ActionResult{
		Clean:    len(failures) == 0,
		Steps:    make([]api.ActionStep, 0, len(stages)),
		Failures: make([]api.ActionStep, 0, len(failures)),
	}
	for _, s := range stages {
		out.Steps = append(out.Steps, api.ActionStep{
			Stage: s.Stage, Status: string(s.Status), Detail: s.Detail,
		})
	}
	for _, f := range failures {
		out.Failures = append(out.Failures, api.ActionStep{
			Stage: f.Stage, Status: string(StageStatusFail), Detail: f.Detail,
		})
	}
	return out
}

// LiveDeployer creates live deployments for callers that are not a cobra
// command (S162b).
//
// # Why it drives the real command rather than reimplementing it
//
// `deploy` is not a thin wrapper. Between the request and the apply sit
// a Layer 3 HCL preflight that denies by default, a credentials check, a
// per-deployment workdir so two deployments cannot share one state file,
// a run-owned project created inside an interrupt guard, and a
// registration step that writes the record even when the apply FAILS --
// because a half-finished apply leaves real resources and the record is
// the only thing that will bring the reaper back to them.
//
// Every one of those is a guard, and every one of them is exactly the
// kind of thing a second implementation gets subtly wrong. So this
// builds the command and calls it, the same way `uiRunStarter` already
// drives `runRunCommand`. The seam is a translation layer; the behaviour
// is the CLI's.
// # Why it builds a runtime per deploy
//
// `CommandRuntime.LoadScenario` CACHES: a runtime that has loaded one
// scenario refuses a different path with "scenario already loaded from
// …". That is right for a CLI process, which handles exactly one
// command, and wrong for a server, which is long-lived. A shared runtime
// would deploy the first scenario asked for and fail every other one.
//
// So the runtime is rebuilt per call, exactly as `uiRunStarter` rebuilds
// one per run. The startup build is kept as well, because it is what
// makes `--allow-deploy` fail loudly at start rather than at the first
// click.
type LiveDeployer struct {
	newRuntime func() (*CommandRuntime, error)

	// scenarioRoot is where a scenario NAME is resolved from. Held so a
	// caller cannot pass a path.
	scenarioRoot string

	// inFlight is the server-side guard against deploying one scenario
	// twice at once.
	//
	// It lives HERE, not in the browser. The page used to be the only
	// thing stopping a second deploy, and a page is exactly the wrong
	// place for that: a refresh wipes it, a second tab never had it, and
	// a `curl` never consulted it. Two deploys of one scenario mean two
	// run-owned projects and two sets of billable resources for one
	// thing.
	mu        sync.Mutex
	deploying map[string]bool
}

// NewLiveDeployer builds the deployer from a runtime factory.
//
// A factory rather than a runtime: see the caching note above.
func NewLiveDeployer(scenarioRoot string, newRuntime func() (*CommandRuntime, error)) *LiveDeployer {
	return &LiveDeployer{
		newRuntime:   newRuntime,
		scenarioRoot: scenarioRoot,
		deploying:    map[string]bool{},
	}
}

// InFlight names the scenarios currently deploying, sorted.
func (d *LiveDeployer) InFlight() []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make([]string, 0, len(d.deploying))
	for scenario := range d.deploying {
		out = append(out, scenario)
	}
	sort.Strings(out)
	return out
}

// claim reserves a scenario, reporting whether it was free.
func (d *LiveDeployer) claim(scenario string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.deploying[scenario] {
		return false
	}
	d.deploying[scenario] = true
	return true
}

func (d *LiveDeployer) release(scenario string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.deploying, scenario)
}

// Deploy applies a scenario and leaves it running under a TTL.
func (d *LiveDeployer) Deploy(ctx context.Context, scenarioName, ttl string, progress io.Writer) (api.ActionResult, error) {
	// A NAME, resolved here, never a path from the caller. `deploy`
	// takes a filesystem path, and accepting one over HTTP would let a
	// request name any YAML on the machine -- including one outside the
	// scenarios tree that the layers have never seen.
	path, err := resolveScenarioByName(d.scenarioRoot, scenarioName)
	if err != nil {
		return api.ActionResult{}, err
	}

	// Claimed AFTER resolution, so a typo cannot lock a name nothing
	// will ever deploy, and BEFORE anything touches the cloud.
	if !d.claim(scenarioName) {
		return api.ActionResult{}, api.ErrDeployInProgress
	}
	// Released whatever happens, including a panic in the command: a
	// scenario stuck marked-as-deploying can never be deployed again
	// without restarting the server, which is a worse failure than the
	// one this prevents.
	defer d.release(scenarioName)

	// Fresh per deploy. A reused runtime has a scenario cached in it and
	// refuses every other one.
	runtime, err := d.newRuntime()
	if err != nil {
		return api.ActionResult{}, err
	}

	cmd := &cobra.Command{Use: "deploy"}
	cmd.Flags().String("ttl", "", "")
	cmd.Flags().String("output", string(OutputModeJSON), "")
	if ttl != "" {
		if err := cmd.Flags().Set("ttl", ttl); err != nil {
			return api.ActionResult{}, err
		}
	}

	// stdout is CAPTURED -- it carries the machine-output contract that
	// deployOutcome parses, and is not the response body.
	//
	// stderr is TEE'd: the command's human progress goes to the caller's
	// writer as it happens, and is also kept so a failure that produced
	// no structured output can still be explained.
	var out, progressCopy strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(deployStderr(progress, &progressCopy))
	cmd.SetContext(ctx)

	deployErr := runDeployCommand(cmd, []string{path}, runtime)

	result := deployOutcome(out.String(), progressCopy.String(), deployErr)
	if deployErr != nil && result.Failures == nil {
		// The command failed before it produced structured output --
		// a usage error, a missing scenario. Surfacing the raw error
		// beats an empty result that reads like nothing went wrong.
		result.Failures = []api.ActionStep{{
			Stage: "deploy", Status: string(StageStatusFail), Detail: deployErr.Error(),
		}}
		result.Clean = false
	}
	return result, nil
}

// deployStderr builds the command's stderr: live to the caller if there
// is one, and always into a copy.
//
// Split out because it is the only place a nil progress writer is
// handled, and a test that never gets past building a runtime cannot
// reach it -- which is exactly what the first attempt at covering this
// did.
//
// io.MultiWriter would store a nil io.Writer and panic on the first
// write, so the nil case must not reach it.
func deployStderr(progress io.Writer, copy io.Writer) io.Writer {
	if progress == nil {
		return copy
	}
	return io.MultiWriter(progress, copy)
}

// deployOutcome turns the command's JSON output into the neutral shape.
//
// A parse failure is reported as a FAILURE rather than swallowed: the
// apply may well have created infrastructure, and an empty result would
// say the opposite.
func deployOutcome(stdout, progress string, deployErr error) api.ActionResult {
	// The WRAPPER, not the result directly. `--output json` emits
	// `{"schema": ..., "result": {...}}`, and unmarshalling that into an
	// OutputResult SUCCEEDS with every field zero -- unknown keys are
	// ignored and `Status` is simply empty. So a successful deploy would
	// have been reported as unclean, with no steps and no failures, and
	// the endpoint would answer 409 after infrastructure was created.
	//
	// A parse that cannot fail is worse than one that does: there is
	// nothing to notice.
	var envelope MachineOutput
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || envelope.Schema == "" {
		if deployErr == nil {
			return api.ActionResult{Clean: false, Failures: []api.ActionStep{{
				Stage: "deploy", Status: string(StageStatusFail),
				Detail: "the deploy finished but its result could not be read, so whether " +
					"infrastructure was created is unknown: " + strings.TrimSpace(progress),
			}}}
		}
		return api.ActionResult{Clean: false}
	}

	payload := envelope.Result
	out := actionResult(payload.Stages, payload.Failures)
	out.Clean = payload.Status == CommandStatusSuccess && len(payload.Failures) == 0
	return out
}

// resolveScenarioByName finds a scenario file by its declared name.
//
// The walk matches on the scenario's `scenario:` field rather than its
// filename, because that is the name the API speaks and the two are not
// required to agree.
func resolveScenarioByName(root, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("no scenario name: %w", os.ErrNotExist)
	}

	var found string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		if ext := filepath.Ext(entry.Name()); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		// Only the NAME is read here, deliberately -- not a validated
		// scenario.
		//
		// `scenario.Load` validates against `DefaultSchemaPath`, which
		// is resolved relative to the working directory. In a server
		// process that is wherever the operator started it, so every
		// file would fail validation and be skipped, and this would
		// report "no scenario named X" for a scenario sitting right
		// there. Worse, it would do so silently.
		//
		// Validation is not skipped, only deferred: `runDeployCommand`
		// loads this path through the runtime's own loader, which knows
		// where the schema is. Resolution needs a name; the command
		// needs a valid scenario, and it checks for itself.
		declared, err := declaredScenarioName(path)
		if err != nil {
			// Unreadable or not YAML at all. Not this call's problem;
			// skipping it beats failing the search for an unrelated
			// scenario, since one broken file in the tree would
			// otherwise make every deploy impossible.
			return nil
		}
		if declared == name {
			found = path
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		// Wrapped so the caller can tell "you asked for something that
		// is not here" from "this server is broken". A client typo, or
		// a UI holding a stale scenario list, is not a 500 -- and
		// reporting it as one teaches operators that 500 means nothing
		// in particular.
		return "", fmt.Errorf("no scenario named %q: %w", name, os.ErrNotExist)
	}
	return found, nil
}

// declaredScenarioName reads only the `scenario:` field.
func declaredScenarioName(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var header struct {
		Scenario string `yaml:"scenario"`
	}
	if err := yaml.Unmarshal(payload, &header); err != nil {
		return "", err
	}
	return header.Scenario, nil
}
