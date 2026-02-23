package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
)

func runRunCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	scenarioPath := args[0]

	controls, err := resolveRunControls(cmd, runtime)
	if err != nil {
		return err
	}
	repairIterationsMax := controls.RepairIterationsMax
	iterationsTarget := controls.IterationsTarget

	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return fmt.Errorf("load scenario %q: %w", scenarioPath, err)
	}
	if runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		result := OutputResult{
			Command:  "run",
			Scenario: sc.Name,
			Status:   CommandStatusFailed,
			Stages: []StageSummary{
				{Layer: "sandbox_deploy", Stage: "blocked", Status: StageStatusSkip, Detail: sandboxBlockedStageDetail()},
			},
			Failures: []FailureSummary{
				{
					Layer:   "sandbox_deploy",
					Stage:   "blocked",
					Check:   "sandbox_deploy",
					Command: "layer gate",
					Detail:  sandboxDeferredDetail(),
				},
			},
		}
		if err := writeCommandOutput(cmd, result); err != nil {
			return err
		}
		return &CLIError{
			Op:   "run",
			Code: errorCodeCommandFailed,
			Err:  fmt.Errorf("sandbox deploy layer is blocked"),
		}
	}

	startedAt := time.Now().UTC()
	runID := startedAt.Format("20060102T150405Z0700")
	store := runstore.NewFilesystemStore(resolveRunStoreRoot())
	logPath := filepath.Join(resolveRunStoreRoot(), sc.Name, runID, "app.log")
	closeRunLogSink, err := runtime.Logger.AddFileSink(logPath)
	if err == nil {
		defer func() {
			_ = closeRunLogSink()
		}()
	}
	runtime.Logger.Log(LogEntry{
		Level:   logLevelInfo,
		Command: "run",
		Event:   "run_start",
		Status:  "start",
		RunID:   runID,
		Detail:  fmt.Sprintf("repair_iterations_max=%d iterations_target=%d", repairIterationsMax, iterationsTarget),
	})

	allStages := make([]StageSummary, 0)
	allFailures := make([]FailureSummary, 0)
	var previousFailures []feedback.Failure
	var previousIterationFailures []FailureSummary
	holdoutBlocked := false
	completed := 0
	terminalReason := ""
	lastIterationFailed := false
	failedIterations := 0
	consecutiveTransportFailures := 0

	for iteration := 1; iteration <= iterationsTarget; iteration++ {
		runtime.Logger.Log(LogEntry{
			Level:     logLevelInfo,
			Command:   "run",
			Event:     "iteration_start",
			Status:    "start",
			RunID:     runID,
			Iteration: iteration,
		})
		completed = iteration
		stages, failures := runIteration(cmd.Context(), runID, iteration, scenarioPath, cmd, runtime, previousIterationFailures)
		allStages = append(allStages, stages...)

		if err := persistRunIteration(store, sc.Name, runID, iteration, stages, failures); err != nil {
			return fmt.Errorf("persist run iteration %d: %w", iteration, err)
		}

		if len(failures) == 0 {
			lastIterationFailed = false
			previousFailures = nil
			previousIterationFailures = previousIterationFailures[:0]
			runtime.Logger.Log(LogEntry{
				Level:     logLevelInfo,
				Command:   "run",
				Event:     "iteration_end",
				Status:    "success",
				RunID:     runID,
				Iteration: iteration,
			})
			continue
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

		if failedIterations > repairIterationsMax {
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

	if terminalReason == "" && lastIterationFailed {
		terminalReason = "repair_budget_exhausted"
		allFailures = append(allFailures, FailureSummary{
			Layer:   "run",
			Stage:   fmt.Sprintf("iteration_%d", completed),
			Check:   "repair_budget_exhausted",
			Command: "run loop",
			Detail:  fmt.Sprintf("reached iterations target (%d) with unresolved failures", iterationsTarget),
		})
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

	if terminalReason == "target_reached" {
		holdoutStages, holdoutFailures, err := runCriteriaOnlyHoldouts(cmd.Context(), runtime, scenarioPath)
		if err != nil {
			return fmt.Errorf("run holdout checks: %w", err)
		}
		allStages = append(allStages, holdoutStages...)
		allFailures = append(allFailures, holdoutFailures...)
		holdoutBlocked = len(holdoutFailures) > 0
	}

	status := CommandStatusSuccess
	if terminalReason != "target_reached" || holdoutBlocked {
		status = CommandStatusFailed
	}

	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Schema:    runstore.RunMetadataSchemaVersion,
		Scenario:  sc.Name,
		RunID:     runID,
		Status:    string(status),
		StartedAt: startedAt,
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

func runIteration(ctx context.Context, runID string, iteration int, scenarioPath string, cmd *cobra.Command, runtime *CommandRuntime, previousIterationFailures []FailureSummary) ([]StageSummary, []FailureSummary) {
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
			testResult, err = executeTest(ctx, runtime, scenarioPath)
		} else {
			switch step.name {
			case "generate":
				_, err = generateAndWriteFiles(ctx, runtime, scenarioPath, iteration, previousIterationFailures)
			case "validate":
				testResult, err = executeValidate(ctx, runtime, scenarioPath)
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
		result, err := executeTestWithScenario(ctx, runtime, sc, runtime.OutputDir())
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
	IterationsTarget    int
}

const transportFailureRetryBudget = 2

func resolveRunControls(cmd *cobra.Command, runtime *CommandRuntime) (runControls, error) {
	repairMax, err := cmd.Flags().GetInt("repair-iterations-max")
	if err != nil {
		return runControls{}, &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf("read --repair-iterations-max flag: %w", err)}
	}

	iterationsTarget, err := cmd.Flags().GetInt("iterations-target")
	if err != nil {
		return runControls{}, &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf("read --iterations-target flag: %w", err)}
	}

	if repairMax == 0 {
		repairMax = runtime.Config.Agent.RepairIterationsMax
	}
	if iterationsTarget == 0 {
		iterationsTarget = runtime.Config.Agent.IterationsTarget
	}

	if repairMax < 1 {
		return runControls{}, &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf("repair iterations max must be >= 1")}
	}
	if iterationsTarget < 1 {
		return runControls{}, &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf("iterations target must be >= 1")}
	}

	return runControls{
		RepairIterationsMax: repairMax,
		IterationsTarget:    iterationsTarget,
	}, nil
}

func toFeedbackFailures(failures []FailureSummary) []feedback.Failure {
	out := make([]feedback.Failure, 0, len(failures))
	for _, failure := range failures {
		out = append(out, feedback.Failure{
			Check:    failure.Check,
			Resource: failure.Resource,
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
