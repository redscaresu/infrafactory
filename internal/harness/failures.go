package harness

import (
	"errors"
	"fmt"
	"strings"

	"github.com/redscaresu/infrafactory/internal/feedback"
)

func StaticFailureFromError(err error) (*feedback.Failure, bool) {
	var stageErr *StageError
	if !errors.As(err, &stageErr) {
		return nil, false
	}

	return &feedback.Failure{
		Layer:   "static",
		Stage:   stageErr.Stage,
		Status:  "fail",
		Check:   stageErr.Stage,
		Command: strings.Join(stageErr.Cmd, " "),
		Detail:  stageFailureDetail(stageErr.Err, stageErr.Stderr),
		Stdout:  stageErr.Stdout,
		Stderr:  stageErr.Stderr,
	}, true
}

func stageFailureDetail(commandErr error, stderr string) string {
	base := commandErr.Error()
	trimmedStderr := strings.TrimSpace(stderr)
	if trimmedStderr == "" {
		return base
	}
	// Keep the surfaced detail compact while preserving actionable stderr text.
	if len(trimmedStderr) > 600 {
		trimmedStderr = trimmedStderr[:600] + "..."
	}
	return fmt.Sprintf("%s | stderr: %s", base, trimmedStderr)
}
