package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/redscaresu/infrafactory/internal/feedback"
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/spf13/cobra"
)

func runRunCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	scenarioPath := args[0]

	maxIterations, err := resolveRunMaxIterations(cmd, runtime)
	if err != nil {
		return err
	}

	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return fmt.Errorf("load scenario %q: %w", scenarioPath, err)
	}

	startedAt := time.Now().UTC()
	runID := startedAt.Format("20060102T150405Z0700")
	store := runstore.NewFilesystemStore(resolveRunStoreRoot())

	allStages := make([]StageSummary, 0)
	allFailures := make([]FailureSummary, 0)
	var previousFailures []feedback.Failure
	converged := false
	completed := 0

	for iteration := 1; iteration <= maxIterations; iteration++ {
		completed = iteration
		stages, failures := runIteration(iteration, scenarioPath, cmd, runtime)
		allStages = append(allStages, stages...)

		if err := persistRunIteration(store, sc.Name, runID, iteration, stages, failures); err != nil {
			return fmt.Errorf("persist run iteration %d: %w", iteration, err)
		}

		if len(failures) == 0 {
			converged = true
			break
		}

		allFailures = append(allFailures, failures...)
		currentFailures := toFeedbackFailures(failures)
		// Stop early when the failure signature is unchanged/subset-equivalent;
		// further iterations are unlikely to produce new signal.
		if feedback.IsStuck(previousFailures, currentFailures) {
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
	}

	if !converged && completed >= maxIterations {
		allFailures = append(allFailures, FailureSummary{
			Layer:   "run",
			Stage:   fmt.Sprintf("iteration_%d", completed),
			Check:   "max_iterations",
			Command: "run loop",
			Detail:  fmt.Sprintf("reached max iterations (%d)", maxIterations),
		})
	}

	status := CommandStatusSuccess
	if !converged {
		status = CommandStatusFailed
	}

	if err := store.WriteRunMetadata(runstore.RunMetadata{
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

	if !converged {
		return &CLIError{
			Op:   "run",
			Code: "command_failed",
			Err:  fmt.Errorf("run did not converge after %d iteration(s)", completed),
		}
	}

	return nil
}

func persistRunIteration(store *runstore.FilesystemStore, scenario string, runID string, iteration int, stages []StageSummary, failures []FailureSummary) error {
	payload, err := json.MarshalIndent(struct {
		Iteration int              `json:"iteration"`
		Stages    []StageSummary   `json:"stages"`
		Failures  []FailureSummary `json:"failures"`
	}{
		Iteration: iteration,
		Stages:    stages,
		Failures:  failures,
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

func runIteration(iteration int, scenarioPath string, cmd *cobra.Command, runtime *CommandRuntime) ([]StageSummary, []FailureSummary) {
	stages := make([]StageSummary, 0, 3)
	failures := make([]FailureSummary, 0)

	steps := []struct {
		name    string
		runner  runtimeHandler
		command string
	}{
		{name: "generate", runner: runGenerateCommand, command: "generate"},
		{name: "validate", runner: runValidateCommand, command: "validate"},
		{name: "test", runner: runTestCommand, command: "test"},
	}

	for _, step := range steps {
		stageName := fmt.Sprintf("iteration_%d_%s", iteration, step.name)
		internalCmd := newInternalRunStepCommand(step.command, cmd)
		err := step.runner(internalCmd, []string{scenarioPath}, runtime)
		if err != nil {
			stages = append(stages, StageSummary{Layer: "run", Stage: stageName, Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer:   "run",
				Stage:   stageName,
				Check:   step.name,
				Command: step.command,
				Detail:  err.Error(),
			})
			break
		}
		stages = append(stages, StageSummary{Layer: "run", Stage: stageName, Status: StageStatusPass})
	}

	return stages, failures
}

func resolveRunMaxIterations(cmd *cobra.Command, runtime *CommandRuntime) (int, error) {
	override, err := cmd.Flags().GetInt("max-iterations")
	if err != nil {
		return 0, &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf("read --max-iterations flag: %w", err)}
	}
	if override > 0 {
		return override, nil
	}
	if runtime.Config.Agent.MaxIterations < 1 {
		return 0, &CLIError{Op: "run", Code: errorCodeUsage, Err: fmt.Errorf("max iterations must be >= 1")}
	}
	return runtime.Config.Agent.MaxIterations, nil
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

func newInternalRunStepCommand(name string, parent *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{Use: name}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.Flags().String("output", string(OutputModeHuman), "")

	if value, err := parent.Flags().GetString("output"); err == nil {
		_ = cmd.Flags().Set("output", value)
	}

	return cmd
}
