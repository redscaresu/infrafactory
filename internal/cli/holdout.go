package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

// holdoutCheckName tags a holdout failure so the run loop can recognise
// one without inspecting its layer or stage.
//
// Recognising it matters for one reason: a holdout failure must NOT
// reach the repair loop. See runHoldouts.
const holdoutCheckName = "holdout"

// runHoldouts probes a RUNNING stack with criteria the generator never
// saw.
//
// # Why this exists
//
// The generator is shown the scenario's acceptance_criteria, and the
// repair loop feeds their failures back, so it optimises against them.
// Passing them proves it satisfied the checks it was given -- not that
// the configuration is right. A holdout is the other half: criteria
// held back, evaluated against the result.
//
// # Why it probes instead of re-running the pipeline
//
// The previous mechanism called executeTestWithScenario, which re-ran
// everything -- mock reset, apply, destroy, and at Layer 3 a whole real
// apply and destroy -- against the SAME output directory. That is both
// expensive and wrong: it re-applies over the state of the stack it is
// meant to be judging, and under `--keep` it would destroy the stack the
// operator asked to preserve.
//
// This reads instead of writes. The stack is already up; the holdout
// dials it.
//
// # Why only probe criteria
//
// `policy` and `destruction` are deliberately unsupported. A policy
// holdout is not really unseen -- the rego applies to every scenario and
// the pitfall corpus already teaches the model about it -- and
// `destruction` is about teardown rather than overfitting. An unsupported
// criterion is REPORTED, not skipped: a holdout that silently drops half
// its checks is the false coverage this whole idea exists to remove.
func runHoldouts(
	ctx context.Context,
	runtime *CommandRuntime,
	scenarioName string,
	workDir string,
) ([]StageSummary, []FailureSummary, bool) {
	holdoutDir := filepath.Join(runtime.Config.Paths.Scenarios, "holdout")
	holdouts, err := scenario.DiscoverCriteriaOnlyHoldouts(holdoutDir, scenarioName)
	if err != nil {
		// A holdout directory that cannot be read is not the same as one
		// with nothing in it, and reporting a pass here would be the
		// "looked at nothing" false green.
		return []StageSummary{{Layer: "holdout", Stage: "discovery", Status: StageStatusFail}},
			[]FailureSummary{{
				Layer: "holdout", Stage: "discovery", Check: holdoutCheckName,
				Command: "holdout discovery",
				Detail:  fmt.Sprintf("could not read %s, so it is unknown whether this scenario has holdouts: %v", holdoutDir, err),
			}}, false
	}

	stages := []StageSummary{{
		Layer: "holdout", Stage: "discovery", Status: StageStatusPass,
		Detail: fmt.Sprintf("%d holdout(s) for %s", len(holdouts), scenarioName),
	}}
	if len(holdouts) == 0 {
		return stages, nil, false
	}

	var failures []FailureSummary
	// Counts CHECKS, not holdout files. A holdout whose every criterion
	// was unprobeable ran nothing, and recording that as "passed" is
	// the false coverage this whole idea exists to remove -- the estate
	// page would tell an operator the unseen checks passed for a
	// deployment nothing ever probed.
	probed := 0
	for _, holdout := range holdouts {
		sc, loadErr := runtime.scenarioLoader(holdout.Path)
		if loadErr != nil {
			stages = append(stages, StageSummary{Layer: "holdout", Stage: holdout.ScenarioName, Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer: "holdout", Stage: holdout.ScenarioName, Check: holdoutCheckName,
				Command: "scenario loader", Detail: loadErr.Error(),
			})
			continue
		}

		checks, unsupported, specErr := holdoutProbeChecks(sc)
		if specErr != nil {
			stages = append(stages, StageSummary{Layer: "holdout", Stage: holdout.ScenarioName, Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer: "holdout", Stage: holdout.ScenarioName, Check: holdoutCheckName,
				Command: "holdout criteria", Detail: specErr.Error(),
			})
			continue
		}
		for _, u := range unsupported {
			stages = append(stages, StageSummary{Layer: "holdout", Stage: holdout.ScenarioName, Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer: "holdout", Stage: holdout.ScenarioName, Check: holdoutCheckName,
				Command: "holdout criteria",
				Detail: fmt.Sprintf(
					"criterion %q cannot be probed against a running stack. A holdout may use connectivity, http_probe or dns_resolution", u),
			})
		}
		if len(checks) == 0 {
			continue
		}

		probed += len(checks)
		// The TRAINING scenario's name, not the holdout's.
		// RealProbeHarness substitutes it into `{{scenario_name}}`, and
		// the DNS records live in the stack the training scenario
		// built -- so passing the holdout's name would resolve
		// `web-live-paris-unseen.example.com` for a record called
		// `web-live-paris.example.com`, and report whatever that
		// lookup happened to do.
		result, probeErr := runtime.Deps.RealProbe.Run(ctx, workDir, scenarioName, checks)
		switch {
		case probeErr != nil:
			stages = append(stages, StageSummary{Layer: "holdout", Stage: holdout.ScenarioName, Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer: "holdout", Stage: holdout.ScenarioName, Check: holdoutCheckName,
				Command: "real probe harness", Detail: probeErr.Error(),
			})
		case len(result.Failures) > 0:
			stages = append(stages, StageSummary{
				Layer: "holdout", Stage: holdout.ScenarioName, Status: StageStatusFail,
				Detail: fmt.Sprintf("%d of %d check(s) failed", len(result.Failures), len(checks)),
			})
			for _, f := range result.Failures {
				failures = append(failures, FailureSummary{
					Layer: "holdout", Stage: holdout.ScenarioName, Check: holdoutCheckName,
					Command:  "real probe harness",
					Resource: f.Resource,
					Detail: fmt.Sprintf(
						"%s -- this check was NOT shown to the generator. Every criterion it was shown passed, so the configuration is fitted to the visible checks rather than correct",
						f.Detail),
				})
			}
		default:
			stages = append(stages, StageSummary{
				Layer: "holdout", Stage: holdout.ScenarioName, Status: StageStatusPass,
				Detail: fmt.Sprintf("%d unseen check(s) passed", len(checks)),
			})
		}
	}

	return stages, failures, probed > 0
}

// holdoutProbeChecks converts a holdout's criteria into probe checks,
// returning the types it cannot handle rather than dropping them.
func holdoutProbeChecks(sc scenario.Scenario) ([]harness.ProbeCheck, []string, error) {
	var checks []harness.ProbeCheck
	var unsupported []string

	specs, err := sc.ExecutableChecks()
	if err != nil {
		return nil, nil, err
	}

	for _, spec := range specs {
		switch spec.Type {
		case "connectivity":
			if spec.Connectivity == nil {
				continue
			}
			checks = append(checks, harness.ProbeCheck{
				Type:   spec.Type,
				Expect: spec.Expect,
				From:   spec.Connectivity.From,
				To:     spec.Connectivity.To,
				Port:   spec.Connectivity.Port,
			})
		case "http_probe":
			if spec.HTTPProbe == nil {
				continue
			}
			checks = append(checks, harness.ProbeCheck{
				Type:   spec.Type,
				Expect: spec.Expect,
				Target: spec.HTTPProbe.Target,
				Port:   spec.HTTPProbe.Port,
			})
		case "dns_resolution":
			if spec.DNSResolution == nil {
				continue
			}
			checks = append(checks, harness.ProbeCheck{
				Type:   spec.Type,
				Expect: spec.Expect,
				Domain: spec.DNSResolution.Domain,
			})
		default:
			unsupported = append(unsupported, spec.Type)
		}
	}

	return checks, unsupported, nil
}

// hasHoldoutFailure reports whether a holdout check failed.
//
// Matched on Check rather than Layer/Stage because runIteration rewrites
// every failure it passes up to the run loop as `Layer: "run"`; Check is
// the field that survives.
func hasHoldoutFailure(failures []FailureSummary) bool {
	for _, f := range failures {
		if f.Check == holdoutCheckName {
			return true
		}
	}
	return false
}

// holdoutStages picks the holdout entries out of a test result.
//
// runIteration collapses everything executeTest reports into one
// `iteration_N_test` line. For most stages that is right -- the detail
// lives in the iteration artifact. Holdouts are the exception: a
// holdout that PASSED would otherwise leave no trace in the run
// summary, and "the checks it was never shown also passed" is the
// entire claim. A result nobody can see is not evidence.
func holdoutStages(stages []StageSummary) []StageSummary {
	var out []StageSummary
	for _, s := range stages {
		if s.Layer == "holdout" {
			out = append(out, s)
		}
	}
	return out
}

// holdoutAfterCriteria probes the running stack, if this run asked for
// it and the stack is known to be serving.
//
// Called from BOTH criteria paths in executeTest -- the one that will
// destroy and the one that will not. `--holdout` is accepted on every
// run, so a run that skips destruction must not silently skip the
// holdout and still report target_reached. One function so the two
// paths cannot drift.
//
// `priorFailures` gates it: a holdout is mostly `expect: blocked`, and
// a blocked check passes TRIVIALLY against a stack that is not serving.
// The scenario's own probe is what waits for the load balancer to come
// up, so running this after an earlier failure would report the
// strongest possible pass for the weakest possible reason.
func holdoutAfterCriteria(
	ctx context.Context,
	runtime *CommandRuntime,
	sc scenario.Scenario,
	opts testExecutionOptions,
	priorFailures []FailureSummary,
) ([]StageSummary, []FailureSummary) {
	if !opts.Holdout {
		return nil, nil
	}
	// Refused at the command, not here (assertHoldoutRunnable), so this
	// is unreachable in practice. Kept as a SKIP rather than a silent
	// nil because a holdout that did not run must never be invisible:
	// the run would report target_reached having probed nothing.
	if !runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return []StageSummary{{
			Layer: "holdout", Stage: "skipped", Status: StageStatusSkip,
			Detail: "no real stack to probe: holdouts need validation.layers.sandbox_deploy.enabled",
		}}, nil
	}
	if len(priorFailures) > 0 {
		return []StageSummary{{
			Layer: "holdout", Stage: "skipped", Status: StageStatusSkip,
			Detail: "earlier checks failed, so the stack is not known to be serving and a blocked-port check would pass for the wrong reason",
		}}, nil
	}
	stages, failures, _ := runHoldouts(ctx, runtime, sc.Name, runtime.OutputDir())
	return stages, failures
}

// holdoutOutcome reads a run's stages for what the holdout actually
// did: "pass", "fail", or empty for did-not-run.
//
// Derived from stages rather than from the --holdout flag, because the
// flag records what was ASKED FOR. A scenario with no holdout files
// produces a clean discovery and no failures, and treating the request
// as the result would put "unseen checks passed" on the estate page for
// a deployment nothing ever probed.
//
// Only the per-holdout stages count. `discovery` reports how many files
// matched -- including zero -- and `skipped` reports that the stack was
// not known to be serving; neither is a check having run.
func holdoutOutcome(stages []StageSummary) string {
	outcome := ""
	for _, s := range stages {
		if s.Layer != "holdout" || s.Stage == "discovery" || s.Stage == "skipped" {
			continue
		}
		if s.Status == StageStatusFail {
			return livestore.HoldoutFail
		}
		if s.Status == StageStatusPass {
			outcome = livestore.HoldoutPass
		}
	}
	return outcome
}

// assertHoldoutRunnable refuses a --holdout this run could not honour.
//
// Checked before generation, for the same reason assertKeepable is: a
// flag that cannot be honoured is a usage error, and usage errors are
// only cheap while they are early. The alternative -- accepting it and
// skipping -- lets a run report target_reached having probed nothing,
// which is the false coverage a holdout exists to remove.
func assertHoldoutRunnable(holdout bool, runtime *CommandRuntime) error {
	if !holdout || runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return nil
	}
	return &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf(
		"--holdout probes a REAL running stack and needs validation.layers.sandbox_deploy.enabled; " +
			"against the mock alone there is nothing to probe")}
}
