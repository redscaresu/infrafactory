package harness

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/redscaresu/infrafactory/internal/feedback"
)

const failureStderrMaxChars = 2000

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

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
	trimmedStderr := strings.TrimSpace(ansiEscapePattern.ReplaceAllString(stderr, ""))
	if trimmedStderr == "" {
		return base
	}
	// Keep the surfaced detail compact while preserving actionable stderr text.
	if len(trimmedStderr) > failureStderrMaxChars {
		trimmedStderr = trimmedStderr[:failureStderrMaxChars] + "..."
	}
	return fmt.Sprintf("%s | stderr: %s", base, trimmedStderr)
}
