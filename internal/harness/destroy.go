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
	var state map[string]any
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return 0, fmt.Errorf("decode state snapshot: %w", err)
	}

	ignoredRoots := map[string]struct{}{
		"metadata": {},
		// fakegcp persists every long-running operation (insert/delete/
		// setLabels/...) to its operations table and surfaces them under
		// state.operations. They aren't resources — they are the audit
		// trail of API calls that produced the resources — so they
		// shouldn't count as orphans on a clean teardown.
		"operations": {},
		// fakeaws (S43-T9) ships two universal bookkeeping tables
		// alongside its service-specific state: `audit` (per-request
		// log) and `schema_version` (the integer schema marker
		// /mock/state emits). Per fakeaws/concepts.md "Required
		// surface" item 7. Service-specific bookkeeping tables append
		// to ignoredCollections in their landing tickets (S46-T4 SQS
		// will add `sqs_messages`).
		"audit":          {},
		"schema_version": {},
	}
	ignoredCollections := map[string]struct{}{
		"events":   {},
		"metrics":  {},
		"messages": {},
	}

	count := 0
	for root, rootNode := range state {
		if _, ignored := ignoredRoots[root]; ignored {
			continue
		}
		rootMap, ok := rootNode.(map[string]any)
		if !ok {
			continue
		}
		for collection, value := range rootMap {
			if _, ignored := ignoredCollections[collection]; ignored {
				continue
			}
			if items, ok := value.([]any); ok {
				count += len(items)
			}
		}
	}

	return count, nil
}
