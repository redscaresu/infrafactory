package feedback

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redscaresu/infrafactory/internal/runstore"
)

type IterationRunner interface {
	RunIteration(context.Context, IterationInput) (IterationResult, error)
}

type IterationInput struct {
	Iteration        int
	PreviousFailures []Failure
}

type IterationResult struct {
	Success  bool      `json:"success"`
	Failures []Failure `json:"failures,omitempty"`
}

type LoopConfig struct {
	MaxIterations int
	Runner        IterationRunner
	Store         *runstore.FilesystemStore
	Scenario      string
	RunID         string
}

type LoopResult struct {
	Converged           bool
	CompletedIterations int
}

func RunLoop(ctx context.Context, cfg LoopConfig) (LoopResult, error) {
	if cfg.MaxIterations < 1 {
		return LoopResult{}, fmt.Errorf("max iterations must be >= 1")
	}
	if cfg.Runner == nil {
		return LoopResult{}, fmt.Errorf("runner is required")
	}
	if cfg.Store == nil {
		return LoopResult{}, fmt.Errorf("store is required")
	}
	if cfg.Scenario == "" || cfg.RunID == "" {
		return LoopResult{}, fmt.Errorf("scenario and run id are required")
	}

	var previousFailures []Failure
	for iteration := 1; iteration <= cfg.MaxIterations; iteration++ {
		outcome, err := cfg.Runner.RunIteration(ctx, IterationInput{
			Iteration:        iteration,
			PreviousFailures: previousFailures,
		})
		if err != nil {
			return LoopResult{CompletedIterations: iteration}, err
		}

		if err := persistIteration(cfg.Store, cfg.Scenario, cfg.RunID, iteration, outcome); err != nil {
			return LoopResult{CompletedIterations: iteration}, err
		}

		if outcome.Success {
			return LoopResult{
				Converged:           true,
				CompletedIterations: iteration,
			}, nil
		}

		previousFailures = outcome.Failures
	}

	return LoopResult{
		Converged:           false,
		CompletedIterations: cfg.MaxIterations,
	}, nil
}

func persistIteration(store *runstore.FilesystemStore, scenario, runID string, iteration int, result IterationResult) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode iteration result: %w", err)
	}

	if err := store.WriteIterationArtifact(scenario, runID, iteration, "iteration.json", payload); err != nil {
		return fmt.Errorf("write iteration artifact: %w", err)
	}

	return nil
}
