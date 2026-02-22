package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrDestroyFailed = errors.New("destroy failed")

type DestroyHarness struct {
	runner CommandRunner
	mock   MockStateClient
}

func NewDestroyHarness(runner CommandRunner, mock MockStateClient) *DestroyHarness {
	return &DestroyHarness{
		runner: runner,
		mock:   mock,
	}
}

type DestroyResult struct {
	Destroy       StageResult
	StateSnapshot []byte
	OrphanCount   int
}

type DestroyError struct {
	Stage   string
	Destroy StageResult
	Err     error
}

func (e *DestroyError) Error() string {
	if e == nil {
		return ErrDestroyFailed.Error()
	}
	return fmt.Sprintf("%s: %s: %v", ErrDestroyFailed, e.Stage, e.Err)
}

func (e *DestroyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *DestroyError) Is(target error) bool {
	return target == ErrDestroyFailed
}

func (h *DestroyHarness) Run(ctx context.Context, workDir string, env map[string]string) (*DestroyResult, error) {
	cmd := Command{
		Name: "tofu",
		Args: []string{"destroy", "-auto-approve"},
		Dir:  workDir,
		Env:  env,
	}
	destroyResult, err := h.runner.Run(ctx, cmd)
	stage := StageResult{
		Stage:  "destroy",
		Cmd:    []string{"tofu", "destroy", "-auto-approve"},
		Stdout: string(destroyResult.Stdout),
		Stderr: string(destroyResult.Stderr),
	}
	if err != nil {
		return nil, &DestroyError{
			Stage:   "destroy",
			Destroy: stage,
			Err:     err,
		}
	}

	stateSnapshot, err := h.mock.State(ctx)
	if err != nil {
		return nil, &DestroyError{
			Stage:   "state",
			Destroy: stage,
			Err:     err,
		}
	}

	orphanCount, err := countOrphans(stateSnapshot)
	if err != nil {
		return nil, &DestroyError{
			Stage:   "orphan_check",
			Destroy: stage,
			Err:     err,
		}
	}
	if orphanCount > 0 {
		return nil, &DestroyError{
			Stage:   "orphan_check",
			Destroy: stage,
			Err:     fmt.Errorf("detected %d orphaned resources", orphanCount),
		}
	}

	return &DestroyResult{
		Destroy:       stage,
		StateSnapshot: stateSnapshot,
		OrphanCount:   orphanCount,
	}, nil
}

func countOrphans(stateJSON []byte) (int, error) {
	var state any
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return 0, fmt.Errorf("decode state snapshot: %w", err)
	}

	return countOrphanNodes(state), nil
}

func countOrphanNodes(node any) int {
	switch typed := node.(type) {
	case []any:
		count := len(typed)
		for _, item := range typed {
			count += countOrphanNodes(item)
		}
		return count
	case map[string]any:
		count := 0
		for _, value := range typed {
			count += countOrphanNodes(value)
		}
		return count
	default:
		return 0
	}
}
