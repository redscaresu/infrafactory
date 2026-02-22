package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrStaticStageFailed = errors.New("static stage failed")

type Command struct {
	Name string
	Args []string
	Dir  string
	Env  map[string]string
}

type CommandResult struct {
	Stdout []byte
	Stderr []byte
}

type CommandRunner interface {
	Run(context.Context, Command) (CommandResult, error)
}

type StaticHarness struct {
	runner CommandRunner
}

func NewStaticHarness(runner CommandRunner) *StaticHarness {
	return &StaticHarness{runner: runner}
}

type StaticResult struct {
	Stages   []StageResult
	PlanJSON []byte
}

type StageResult struct {
	Stage  string
	Cmd    []string
	Stdout string
	Stderr string
}

type StageError struct {
	StageResult
	Err error
}

func (e *StageError) Error() string {
	if e == nil {
		return ErrStaticStageFailed.Error()
	}
	return fmt.Sprintf("%s: %s: %v", ErrStaticStageFailed, e.Stage, e.Err)
}

func (e *StageError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *StageError) Is(target error) bool {
	return target == ErrStaticStageFailed
}

func (h *StaticHarness) Run(ctx context.Context, workDir string, env map[string]string) (*StaticResult, error) {
	// Stage order is fixed to keep outputs deterministic and to ensure each
	// command consumes artifacts from the expected prior stage.
	stages := []struct {
		name string
		cmd  Command
	}{
		{
			name: "init",
			cmd: Command{
				Name: "tofu",
				Args: []string{"init"},
				Dir:  workDir,
				Env:  env,
			},
		},
		{
			name: "validate",
			cmd: Command{
				Name: "tofu",
				Args: []string{"validate"},
				Dir:  workDir,
				Env:  env,
			},
		},
		{
			name: "plan",
			cmd: Command{
				Name: "tofu",
				Args: []string{"plan", "-out=tfplan"},
				Dir:  workDir,
				Env:  env,
			},
		},
		{
			name: "show",
			cmd: Command{
				Name: "tofu",
				Args: []string{"show", "-json", "tfplan"},
				Dir:  workDir,
				Env:  env,
			},
		},
	}

	result := &StaticResult{
		Stages: make([]StageResult, 0, len(stages)),
	}

	for _, stage := range stages {
		cmdResult, err := h.runner.Run(ctx, stage.cmd)
		stageResult := StageResult{
			Stage:  stage.name,
			Cmd:    append([]string{stage.cmd.Name}, stage.cmd.Args...),
			Stdout: string(cmdResult.Stdout),
			Stderr: string(cmdResult.Stderr),
		}
		result.Stages = append(result.Stages, stageResult)

		if err != nil {
			return result, &StageError{
				StageResult: stageResult,
				Err:         err,
			}
		}

		if stage.name == "show" {
			if err := validateJSON(cmdResult.Stdout); err != nil {
				return result, &StageError{
					StageResult: stageResult,
					Err:         fmt.Errorf("invalid plan json: %w", err),
				}
			}
			result.PlanJSON = cmdResult.Stdout
		}
	}

	return result, nil
}

func validateJSON(payload []byte) error {
	var body any
	return json.Unmarshal(payload, &body)
}
