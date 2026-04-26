package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
)

type runIDContextKey struct{}

type runMode string

const (
	runModeClean       runMode = "clean"
	runModeIncremental runMode = "incremental"
)

type detectedRunMode struct {
	Mode          runMode
	Reason        string
	PreviousRunID string
	BaselineState []byte
}

func runRunCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	scenarioPath := args[0]

	controls, err := resolveRunControls(cmd, runtime)
	if err != nil {
		return err
	}
	repairIterationsMax := controls.RepairIterationsMax

	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return fmt.Errorf("load scenario %q: %w", scenarioPath, err)
	}

	startedAt := time.Now().UTC()
	runID, _ := cmd.Context().Value(runIDContextKey{}).(string)
	if runID == "" {
		runID = startedAt.Format("20060102T150405Z0700")
	}
	store := runstore.NewFilesystemStore(resolveRunStoreRoot())
	mode, err := detectRunMode(cmd.Context(), runtime, store, sc.Name, runtime.OutputDir(), controls)
	if err != nil {
		return err
	}
	if mode.Mode == runModeIncremental && len(mode.BaselineState) > 0 {
		if err := store.WriteRunArtifact(sc.Name, runID, "baseline_state.json", mode.BaselineState); err != nil {
			return fmt.Errorf("write baseline state artifact: %w", err)
		}
	}
	if mode.Mode == runModeIncremental {
		if err := runtime.Deps.MockState.Snapshot(cmd.Context()); err != nil {
			return fmt.Errorf("snapshot mockway baseline: %w", err)
		}
	}
	logPath := filepath.Join(resolveRunStoreRoot(), sc.Name, runID, "app.log")
	closeRunLogSink, err := runtime.Logger.AddFileSink(logPath)
	if err == nil {
		defer func() {
			_ = closeRunLogSink()
		}()
	} else {
		runtime.Logger.Log(LogEntry{
			Level:   logLevelError,
			Command: "run",
			Event:   "run_log_sink_unavailable",
			Status:  "warn",
			RunID:   runID,
			Detail:  fmt.Sprintf("path=%s error=%v", logPath, err),
		})
	}
	runtime.Logger.Log(LogEntry{
		Level:   logLevelInfo,
		Command: "run",
		Event:   "run_start",
		Status:  "start",
		RunID:   runID,
		Detail:  fmt.Sprintf("repair_iterations_max=%d run_mode=%s no_destroy=%t previous_run_id=%s", repairIterationsMax, mode.Mode, controls.NoDestroy, mode.PreviousRunID),
	})
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Schema:              runstore.RunMetadataSchemaVersion,
		Scenario:            sc.Name,
		RunID:               runID,
		Status:              "running",
		Incremental:         mode.Mode == runModeIncremental,
		Layer3Enabled:       runtime.Config.Validation.Layers.SandboxDeploy.Enabled,
		PreviousRunID:       mode.PreviousRunID,
		RepairIterationsMax: repairIterationsMax,
		StartedAt:           startedAt,
	}); err != nil {
		return fmt.Errorf("write initial run metadata: %w", err)
	}

	allStages := []StageSummary{
		{Layer: "run", Stage: "mode", Status: StageStatusPass, Detail: fmt.Sprintf("%s (%s)", mode.Mode, mode.Reason)},
	}
	allFailures := make([]FailureSummary, 0)
	var previousFailures []feedback.Failure
	var previousIterationFailures []FailureSummary
	var iterationHistory []feedback.IterationResult
	holdoutBlocked := false
	completed := 0
	terminalReason := ""
	lastIterationFailed := false
	failedIterations := 0
	consecutiveTransportFailures := 0
	captureLLMRaw := llmRawCaptureEnabled()

	for iteration := 1; ; iteration++ {
		runtime.Logger.Log(LogEntry{
			Level:     logLevelInfo,
			Command:   "run",
			Event:     "iteration_start",
			Status:    "start",
			RunID:     runID,
			Iteration: iteration,
		})
		completed = iteration
		stages, failures := runIteration(cmd.Context(), runID, iteration, sc.Name, scenarioPath, runtime, store, captureLLMRaw, previousIterationFailures, mode.Mode, controls.NoDestroy)
		allStages = append(allStages, stages...)

		if err := persistRunIteration(store, sc.Name, runID, iteration, stages, failures); err != nil {
			return fmt.Errorf("persist run iteration %d: %w", iteration, err)
		}

		if len(failures) == 0 {
			// Auto-learn pitfalls: if this is iteration 2+, a self-correction
			// happened. Extract actionable patterns from the previous iteration's
			// failures and persist them for future runs. Mirrors the oscillation
			// path's cross-cloud isolation — a GCP scenario whose detail had no
			// resource name would otherwise pick up a Scaleway-flavoured fallback
			// from ExtractLearnedPitfall and pollute pitfalls/gcp.yaml.
			if iteration > 1 && len(previousIterationFailures) > 0 {
				cloud := sc.Cloud
				for _, failure := range previousIterationFailures {
					learned := generator.ExtractLearnedPitfall(failure.Detail, sc.Name)
					if learned == nil {
						continue
					}
					if !pitfallResourceMatchesCloud(learned.Resource, cloud) {
						continue
					}
					if err := generator.AppendPitfall(runtime.Config.Paths.Pitfalls, cloud, *learned); err != nil {
						runtime.Logger.Log(LogEntry{
							Level:   logLevelError,
							Command: "run",
							Event:   "self_correction_pitfall_append",
							Status:  "failed",
							RunID:   runID,
							Detail:  err.Error(),
						})
					}
				}
			}
			lastIterationFailed = false
			previousFailures = nil
			previousIterationFailures = previousIterationFailures[:0]
			terminalReason = "target_reached"
			runtime.Logger.Log(LogEntry{
				Level:     logLevelInfo,
				Command:   "run",
				Event:     "iteration_end",
				Status:    "success",
				RunID:     runID,
				Iteration: iteration,
			})
			break
		}

		lastIterationFailed = true
		failedIterations++
		allFailures = append(allFailures, failures...)
		transportDominated := failuresAreTransportDominated(failures)
		if transportDominated {
			consecutiveTransportFailures++
		} else {
			consecutiveTransportFailures = 0
		}
		runtime.Logger.Log(LogEntry{
			Level:     logLevelError,
			Command:   "run",
			Event:     "iteration_end",
			Status:    "failed",
			RunID:     runID,
			Iteration: iteration,
			Detail:    fmt.Sprintf("%d failure(s)", len(failures)),
		})
		currentFailures := toFeedbackFailures(failures)
		iterationHistory = append(iterationHistory, feedback.IterationResult{
			Iteration: iteration,
			Failures:  currentFailures,
		})
		if consecutiveTransportFailures >= transportFailureRetryBudget {
			terminalReason = "repair_budget_exhausted"
			allFailures = append(allFailures, FailureSummary{
				Layer:   "run",
				Stage:   fmt.Sprintf("iteration_%d", iteration),
				Check:   "transport_runtime_dominated",
				Command: "run loop",
				Detail:  fmt.Sprintf("stopped early after %d consecutive transport-runtime failures; consider adjusting agent timeouts/retries or fixing transport dependencies", consecutiveTransportFailures),
			})
			break
		}
		// Stop early when the failure signature is unchanged/subset-equivalent;
		// further iterations are unlikely to produce new signal.
		if feedback.IsStuck(previousFailures, currentFailures) {
			terminalReason = "stuck"
			allFailures = append(allFailures, FailureSummary{
				Layer:   "run",
				Stage:   fmt.Sprintf("iteration_%d", iteration),
				Check:   "stuck",
				Command: "run loop",
				Detail:  "stopped due to stuck detection",
			})
			break
		}
		previousFailures = currentFailures
		previousIterationFailures = append(previousIterationFailures[:0], failures...)

		if failedIterations >= repairIterationsMax {
			terminalReason = "repair_budget_exhausted"
			allFailures = append(allFailures, FailureSummary{
				Layer:   "run",
				Stage:   fmt.Sprintf("iteration_%d", iteration),
				Check:   "repair_budget_exhausted",
				Command: "run loop",
				Detail:  fmt.Sprintf("reached repair iterations max (%d)", repairIterationsMax),
			})
			break
		}
	}

	if terminalReason == "" && !lastIterationFailed {
		terminalReason = "target_reached"
	}
	runtime.Logger.Log(LogEntry{
		Level:   logLevelInfo,
		Command: "run",
		Event:   "terminal_reason",
		Status:  terminalReason,
		RunID:   runID,
		Detail:  fmt.Sprintf("completed_iterations=%d", completed),
	})

	// Auto-learn from oscillation failures: when a run exhausts its repair
	// budget or stops on stuck detection, scan the iteration history for
	// failure signatures that oscillated (present-N, absent-N+1,
	// present-N+2). These are stable indicators of the model alternating
	// between two incomplete fixes; their detail strings are exactly the
	// pitfall material we want future runs to inherit.
	if terminalReason == "repair_budget_exhausted" || terminalReason == "stuck" {
		oscillating := feedback.DetectOscillation(iterationHistory)
		for _, sig := range oscillating {
			learned := generator.ExtractLearnedPitfall(sig.Detail, sc.Name)
			if learned == nil {
				continue
			}
			// ExtractLearnedPitfall has Scaleway-flavoured fallbacks
			// (scaleway_redis_cluster, scaleway_k8s_cluster) that fire
			// when the failure detail names no resource. Don't pollute
			// pitfalls/<otherCloud>.yaml with cross-cloud resource
			// types; skip when the learned resource prefix doesn't
			// match the scenario's cloud.
			if !pitfallResourceMatchesCloud(learned.Resource, sc.Cloud) {
				runtime.Logger.Log(LogEntry{
					Level:   logLevelInfo,
					Command: "run",
					Event:   "oscillation_pitfall_skipped",
					Status:  "skipped",
					RunID:   runID,
					Detail:  fmt.Sprintf("cross-cloud resource %q for cloud=%s", learned.Resource, sc.Cloud),
				})
				continue
			}
			if err := generator.AppendPitfall(runtime.Config.Paths.Pitfalls, sc.Cloud, *learned); err != nil {
				runtime.Logger.Log(LogEntry{
					Level:   logLevelError,
					Command: "run",
					Event:   "oscillation_pitfall_append",
					Status:  "failed",
					RunID:   runID,
					Detail:  err.Error(),
				})
				continue
			}
			runtime.Logger.Log(LogEntry{
				Level:   logLevelInfo,
				Command: "run",
				Event:   "oscillation_pitfall_learned",
				Status:  "success",
				RunID:   runID,
				Detail:  fmt.Sprintf("resource=%s rule=%s", learned.Resource, learned.Rule),
			})
		}
	}

	// Auto-destroy real Scaleway resources on failure to prevent orphaned billing.
	// Contract #14: failed run without --no-destroy must destroy real resources.
	sandboxEnabled := runtime.Config.Validation.Layers.SandboxDeploy.Enabled
	if sandboxEnabled && !controls.NoDestroy && terminalReason != "target_reached" {
		liveStatePath := filepath.Join(runtime.OutputDir(), harness.LiveStateFilename)
		if _, statErr := os.Stat(liveStatePath); statErr == nil {
			sandboxEnv, sandboxEnvErr := sandboxCommandEnv(runtime)
			if sandboxEnvErr != nil {
				runtime.Logger.Log(LogEntry{
					Level:   logLevelError,
					Command: "run",
					Event:   "layer3_auto_destroy_preflight",
					Status:  "failed",
					RunID:   runID,
					Detail:  sandboxEnvErr.Error(),
				})
				allStages = append(allStages, StageSummary{Layer: "sandbox_deploy", Stage: "auto_destroy_preflight", Status: StageStatusFail})
			} else {
				destroyResult, destroyErr := runtime.Deps.SandboxDestroy.Run(cmd.Context(), runtime.OutputDir(), sandboxEnv)
				destroyStages, destroyFailures := appendSandboxDestroyResult(nil, nil, destroyResult, destroyErr)
				allStages = append(allStages, destroyStages...)
				if destroyErr != nil {
					runtime.Logger.Log(LogEntry{
						Level:   logLevelError,
						Command: "run",
						Event:   "layer3_auto_destroy",
						Status:  "failed",
						RunID:   runID,
						Detail:  destroyErr.Error(),
					})
					allFailures = append(allFailures, destroyFailures...)
				} else {
					runtime.Logger.Log(LogEntry{
						Level:   logLevelInfo,
						Command: "run",
						Event:   "layer3_auto_destroy",
						Status:  "success",
						RunID:   runID,
					})
				}
			}
		}
	}

	if terminalReason == "target_reached" && !controls.NoDestroy {
		holdoutStages, holdoutFailures, err := runCriteriaOnlyHoldouts(cmd.Context(), runtime, scenarioPath)
		if err != nil {
			return fmt.Errorf("run holdout checks: %w", err)
		}
		allStages = append(allStages, holdoutStages...)
		allFailures = append(allFailures, holdoutFailures...)
		holdoutBlocked = len(holdoutFailures) > 0
	} else if terminalReason == "target_reached" && controls.NoDestroy {
		allStages = append(allStages, StageSummary{Layer: "holdout", Stage: "skipped", Status: StageStatusSkip, Detail: "skipped by --no-destroy"})
	}

	status := CommandStatusSuccess
	if terminalReason != "target_reached" || holdoutBlocked {
		status = CommandStatusFailed
	}

	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Schema:              runstore.RunMetadataSchemaVersion,
		Scenario:            sc.Name,
		RunID:               runID,
		Status:              string(status),
		TerminalReason:      terminalReason,
		Incremental:         mode.Mode == runModeIncremental,
		Layer3Enabled:       runtime.Config.Validation.Layers.SandboxDeploy.Enabled,
		PreviousRunID:       mode.PreviousRunID,
		RepairIterationsMax: repairIterationsMax,
		StartedAt:           startedAt,
	}); err != nil {
		return fmt.Errorf("write run metadata: %w", err)
	}

	allStages = append(allStages, StageSummary{
		Layer:  "run",
		Stage:  "metadata",
		Status: StageStatusPass,
		Detail: fmt.Sprintf("run_id=%s", runID),
	})
	if terminalReason != "" {
		allStages = append(allStages, StageSummary{
			Layer:  "run",
			Stage:  "terminal_reason",
			Status: StageStatusPass,
			Detail: terminalReason,
		})
	}

	result := OutputResult{
		Command:  "run",
		Scenario: sc.Name,
		Status:   status,
		Stages:   allStages,
		Failures: allFailures,
	}
	if err := writeCommandOutput(cmd, result); err != nil {
		return err
	}

	if terminalReason != "target_reached" || holdoutBlocked {
		errDetail := fmt.Errorf("run stopped with terminal reason %q after %d iteration(s)", terminalReason, completed)
		if holdoutBlocked {
			errDetail = fmt.Errorf("holdout checks failed after training convergence")
		}
		return &CLIError{
			Op:   "run",
			Code: errorCodeCommandFailed,
			Err:  errDetail,
		}
	}

	return nil
}

func persistRunIteration(store *runstore.FilesystemStore, scenario string, runID string, iteration int, stages []StageSummary, failures []FailureSummary) error {
	failureSummary := make([]string, 0, len(failures))
	transportDiagnostics := make([]map[string]string, 0)
	for _, failure := range failures {
		failureSummary = append(failureSummary, fmt.Sprintf("%s/%s check=%s command=%s detail=%s", failure.Layer, failure.Stage, failure.Check, failure.Command, failure.Detail))
		if feedbackFailureClassForSummary(failure) == "transport_runtime" {
			transportDiagnostics = append(transportDiagnostics, map[string]string{
				"stage":   failure.Stage,
				"check":   failure.Check,
				"command": failure.Command,
				"detail":  failure.Detail,
			})
		}
	}

	payload, err := json.MarshalIndent(struct {
		Schema               string              `json:"schema"`
		Iteration            int                 `json:"iteration"`
		Stages               []StageSummary      `json:"stages"`
		Failures             []FailureSummary    `json:"failures"`
		FailureSummary       []string            `json:"failure_summary,omitempty"`
		TransportDiagnostics []map[string]string `json:"transport_diagnostics,omitempty"`
	}{
		Schema:               runstore.RunIterationSchemaVersion,
		Iteration:            iteration,
		Stages:               stages,
		Failures:             failures,
		FailureSummary:       failureSummary,
		TransportDiagnostics: transportDiagnostics,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode iteration artifact: %w", err)
	}

	if err := store.WriteIterationArtifact(scenario, runID, iteration, "iteration.json", payload); err != nil {
		return fmt.Errorf("write iteration artifact: %w", err)
	}

	return nil
}

func resolveRunStoreRoot() string {
	if root := os.Getenv("INFRAFACTORY_RUNSTORE_ROOT"); root != "" {
		return root
	}
	return runstore.DefaultRoot
}

func runIteration(
	ctx context.Context,
	runID string,
	iteration int,
	scenarioName string,
	scenarioPath string,
	runtime *CommandRuntime,
	store *runstore.FilesystemStore,
	captureLLMRaw bool,
	previousIterationFailures []FailureSummary,
	mode runMode,
	noDestroy bool,
) ([]StageSummary, []FailureSummary) {
	stages := make([]StageSummary, 0, 3)
	failures := make([]FailureSummary, 0)

	steps := []struct {
		name    string
		command string
	}{
		{name: "generate", command: "generate"},
		{name: "validate", command: "validate"},
		{name: "test", command: "test"},
	}

	for _, step := range steps {
		stageName := fmt.Sprintf("iteration_%d_%s", iteration, step.name)
		runtime.Logger.Log(LogEntry{
			Level:     logLevelInfo,
			Command:   "run",
			Event:     "stage_start",
			Status:    "start",
			RunID:     runID,
			Iteration: iteration,
			Stage:     stageName,
		})
		var (
			err        error
			testResult OutputResult
		)
		if step.name == "test" {
			testResult, err = executeTest(ctx, runtime, scenarioPath, testExecutionOptions{
				MockDeployMode: mockDeployModeForRunMode(mode),
				SkipDestroy:    noDestroy,
			})
			if len(testResult.PlanLiveText) > 0 {
				writeErr := store.WriteIterationArtifact(scenarioName, runID, iteration, "plan-live.txt", testResult.PlanLiveText)
				if writeErr != nil && err == nil {
					err = writeErr
				}
			}
		} else {
			switch step.name {
			case "generate":
				var generated *generator.GeneratedCode
				_, generated, err = generateAndWriteFilesWithResult(ctx, runtime, scenarioPath, iteration, previousIterationFailures, generatedFileWriteModeForRunMode(mode))
				if err == nil {
					err = store.WriteGeneratedFiles(scenarioName, runID, generated.Files)
				}
				if err == nil {
					err = store.WriteIterationGeneratedFiles(scenarioName, runID, iteration, generated.Files)
				}
				if err == nil && captureLLMRaw {
					err = persistLLMRawPhaseResponses(store, scenarioName, runID, iteration, generated.Metadata.Phases)
				}
			case "validate":
				var artifacts validateArtifacts
				testResult, artifacts, err = executeValidateWithArtifacts(ctx, runtime, scenarioPath)
				if len(artifacts.PlanText) > 0 {
					writeErr := store.WriteRunArtifact(scenarioName, runID, "plan.txt", artifacts.PlanText)
					if writeErr != nil && err == nil {
						err = writeErr
					}
				}
			default:
				err = fmt.Errorf("unsupported run step %q", step.name)
			}
		}
		if err != nil {
			stages = append(stages, StageSummary{Layer: "run", Stage: stageName, Status: StageStatusFail})
			if (step.name == "test" || step.name == "validate") && len(testResult.Failures) > 0 {
				for _, failure := range testResult.Failures {
					failures = append(failures, FailureSummary{
						Layer:    "run",
						Stage:    stageName,
						Check:    failure.Check,
						Policy:   failure.Policy,
						Command:  step.command,
						Resource: failure.Resource,
						Detail:   failure.Detail,
					})
				}
			} else {
				failures = append(failures, FailureSummary{
					Layer:   "run",
					Stage:   stageName,
					Check:   step.name,
					Command: step.command,
					Detail:  err.Error(),
				})
			}
			runtime.Logger.Log(LogEntry{
				Level:     logLevelError,
				Command:   "run",
				Event:     "stage_end",
				Status:    "failed",
				RunID:     runID,
				Iteration: iteration,
				Stage:     stageName,
				Check:     step.name,
				Detail:    err.Error(),
			})
			break
		}
		stages = append(stages, StageSummary{Layer: "run", Stage: stageName, Status: StageStatusPass})
		runtime.Logger.Log(LogEntry{
			Level:     logLevelInfo,
			Command:   "run",
			Event:     "stage_end",
			Status:    "success",
			RunID:     runID,
			Iteration: iteration,
			Stage:     stageName,
		})
	}

	return stages, failures
}

func runCriteriaOnlyHoldouts(ctx context.Context, runtime *CommandRuntime, trainingScenarioPath string) ([]StageSummary, []FailureSummary, error) {
	holdoutDir := filepath.Join(runtime.Config.Paths.Scenarios, "holdout")
	holdouts, err := scenario.DiscoverCriteriaOnlyHoldouts(holdoutDir, trainingScenarioPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []StageSummary{{Layer: "holdout", Stage: "discovery", Status: StageStatusPass, Detail: "0 holdouts"}}, nil, nil
		}
		return nil, nil, err
	}

	stages := []StageSummary{
		{
			Layer:  "holdout",
			Stage:  "discovery",
			Status: StageStatusPass,
			Detail: fmt.Sprintf("%d holdouts", len(holdouts)),
		},
	}
	failures := make([]FailureSummary, 0)
	for _, holdout := range holdouts {
		sc, err := runtime.scenarioLoader(holdout.Path)
		if err != nil {
			stages = append(stages, StageSummary{Layer: "holdout", Stage: holdout.ScenarioName, Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer:   "holdout",
				Stage:   holdout.ScenarioName,
				Check:   "load",
				Command: "scenario loader",
				Detail:  err.Error(),
			})
			continue
		}

		// Criteria-only holdouts validate against the generated code of the
		// already-converged training scenario, not their own output path.
		result, err := executeTestWithScenario(ctx, runtime, sc, runtime.OutputDir(), testExecutionOptions{MockDeployMode: harness.MockDeployModeClean})
		if err != nil {
			stages = append(stages, StageSummary{Layer: "holdout", Stage: holdout.ScenarioName, Status: StageStatusFail})
			if len(result.Failures) == 0 {
				failures = append(failures, FailureSummary{
					Layer:   "holdout",
					Stage:   holdout.ScenarioName,
					Check:   "test",
					Command: "test",
					Detail:  err.Error(),
				})
				continue
			}
			for _, failure := range result.Failures {
				failures = append(failures, FailureSummary{
					Layer:    "holdout",
					Stage:    holdout.ScenarioName,
					Check:    failure.Check,
					Policy:   failure.Policy,
					Command:  "test",
					Resource: failure.Resource,
					Detail:   failure.Detail,
				})
			}
			continue
		}

		stage := StageSummary{Layer: "holdout", Stage: holdout.ScenarioName, Status: StageStatusPass}
		for _, testStage := range result.Stages {
			if testStage.Layer == "criteria" && testStage.Stage == "support_matrix" && testStage.Detail != "" {
				stage.Detail = testStage.Detail
				break
			}
		}
		stages = append(stages, stage)
	}

	return stages, failures, nil
}

type runControls struct {
	RepairIterationsMax int
	Clean               bool
	NoDestroy           bool
}

const transportFailureRetryBudget = 2

func resolveRunControls(cmd *cobra.Command, runtime *CommandRuntime) (runControls, error) {
	repairMax, err := cmd.Flags().GetInt("repair-iterations-max")
	if err != nil {
		return runControls{}, &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf("read --repair-iterations-max flag: %w", err)}
	}
	clean, err := cmd.Flags().GetBool("clean")
	if err != nil {
		return runControls{}, &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf("read --clean flag: %w", err)}
	}
	noDestroy, err := cmd.Flags().GetBool("no-destroy")
	if err != nil {
		return runControls{}, &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf("read --no-destroy flag: %w", err)}
	}
	if clean && noDestroy {
		return runControls{}, &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf("clean and no-destroy are mutually exclusive")}
	}

	if repairMax == 0 {
		repairMax = runtime.Config.Agent.RepairIterationsMax
	}

	if repairMax < 1 {
		return runControls{}, &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf("repair iterations max must be >= 1")}
	}

	return runControls{
		RepairIterationsMax: repairMax,
		Clean:               clean,
		NoDestroy:           noDestroy,
	}, nil
}

func detectRunMode(ctx context.Context, runtime *CommandRuntime, store *runstore.FilesystemStore, scenarioName, outputDir string, controls runControls) (detectedRunMode, error) {
	if controls.Clean {
		statePayload, err := runtime.Deps.MockState.State(ctx)
		if err != nil {
			return detectedRunMode{}, fmt.Errorf("capture baseline state for clean run: %w", err)
		}
		return detectedRunMode{Mode: runModeClean, Reason: "forced by --clean", BaselineState: statePayload}, nil
	}

	statePayload, err := runtime.Deps.MockState.State(ctx)
	if err != nil {
		return detectedRunMode{}, fmt.Errorf("detect run mode from mockway state: %w", err)
	}
	hasMockResources, err := mockStateHasResources(statePayload)
	if err != nil {
		return detectedRunMode{}, fmt.Errorf("decode mockway state for run mode detection: %w", err)
	}
	hasTFState := tfStateExists(outputDir)
	previousRunID, err := store.LatestSuccessfulRunID(scenarioName)
	if err != nil {
		return detectedRunMode{}, fmt.Errorf("detect previous successful run: %w", err)
	}

	missing := make([]string, 0, 3)
	if !hasMockResources {
		missing = append(missing, "mockway state")
	}
	if !hasTFState {
		missing = append(missing, "terraform.tfstate")
	}
	if previousRunID == "" {
		missing = append(missing, "previous successful run")
	}
	if len(missing) == 0 {
		return detectedRunMode{
			Mode:          runModeIncremental,
			Reason:        "auto-detected from mockway state, terraform.tfstate, and previous successful run",
			PreviousRunID: previousRunID,
			BaselineState: statePayload,
		}, nil
	}

	return detectedRunMode{
		Mode:          runModeClean,
		Reason:        "missing " + strings.Join(missing, ", "),
		BaselineState: statePayload,
	}, nil
}

func mockStateHasResources(payload []byte) (bool, error) {
	var state map[string]any
	if err := json.Unmarshal(payload, &state); err != nil {
		return false, err
	}
	for _, rootNode := range state {
		rootMap, ok := rootNode.(map[string]any)
		if !ok {
			continue
		}
		for _, value := range rootMap {
			items, ok := value.([]any)
			if ok && len(items) > 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

func tfStateExists(outputDir string) bool {
	_, err := os.Stat(filepath.Join(outputDir, "terraform.tfstate"))
	return err == nil
}

func mockDeployModeForRunMode(mode runMode) harness.MockDeployMode {
	if mode == runModeIncremental {
		return harness.MockDeployModeIncremental
	}
	return harness.MockDeployModeClean
}

func generatedFileWriteModeForRunMode(mode runMode) generatedFileWriteMode {
	if mode == runModeIncremental {
		return generatedFileWriteModeIncremental
	}
	return generatedFileWriteModeClean
}

// pitfallResourceMatchesCloud reports whether a learned pitfall's
// resource name fits the scenario's cloud. Empty cloud and empty
// resource accept anything (no signal to mismatch). Otherwise the
// resource must be prefixed with the cloud's canonical Terraform
// provider stem (scaleway_*, google_*).
func pitfallResourceMatchesCloud(resource, cloud string) bool {
	if cloud == "" || resource == "" {
		return true
	}
	switch cloud {
	case "scaleway":
		return strings.HasPrefix(resource, "scaleway_")
	case "gcp":
		return strings.HasPrefix(resource, "google_")
	default:
		return true
	}
}

func toFeedbackFailures(failures []FailureSummary) []feedback.Failure {
	out := make([]feedback.Failure, 0, len(failures))
	for _, failure := range failures {
		out = append(out, feedback.Failure{
			Check:    failure.Check,
			Resource: failure.Resource,
			Detail:   failure.Detail,
		})
	}
	return out
}

func failuresAreTransportDominated(failures []FailureSummary) bool {
	if len(failures) == 0 {
		return false
	}
	for _, failure := range failures {
		if feedbackFailureClassForSummary(failure) != "transport_runtime" {
			return false
		}
	}
	return true
}
