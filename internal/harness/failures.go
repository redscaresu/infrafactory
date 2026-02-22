package harness

import (
	"errors"
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
		Detail:  stageErr.Err.Error(),
		Stdout:  stageErr.Stdout,
		Stderr:  stageErr.Stderr,
	}, true
}
