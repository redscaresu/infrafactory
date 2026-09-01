package cli

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/redscaresu/infrafactory/internal/api"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

// LiveActions performs teardown and reap for callers that are not a
// cobra command -- today, the UI server (S159b).
//
// It exists so those callers reach the SAME code the CLI does rather
// than a second implementation. Teardown is the guard between an
// automated action and somebody's real infrastructure: it refuses unless
// a run-owned marker and the API's own provenance BOTH say the project
// is infrafactory's, and it declines to mark a deployment released when
// its state has vanished, because doing so would retire the only record
// saying the resources may still be running.
//
// None of that is re-expressed here. `tearDownDeployment` already takes
// a context, a runtime and a store, so this type is a translation layer
// and nothing else -- which is the point. A second implementation of a
// guard is a second thing that can be wrong.
type LiveActions struct {
	runtime *CommandRuntime
}

// NewLiveActions builds the actor from an already-constructed runtime.
func NewLiveActions(runtime *CommandRuntime) *LiveActions {
	return &LiveActions{runtime: runtime}
}

func (l *LiveActions) store() *livestore.FilesystemStore {
	return livestore.NewFilesystemStore(l.runtime.LiveStoreRoot())
}

// Teardown destroys one deployment, by id.
func (l *LiveActions) Teardown(ctx context.Context, id string) (api.ActionResult, error) {
	store := l.store()

	d, err := store.Get(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.ActionResult{}, err
		}
		// An undecodable record still reaches teardown, which reports it
		// as unreclaimable and names `live forget`. The CLI does the
		// same; refusing here would hide a record that may describe
		// running infrastructure.
		d = livestore.Deployment{ID: id, Undecodable: true}
	}

	stages, failures := tearDownDeployment(ctx, l.runtime, store, d)
	return actionResult(stages, failures), nil
}

// Reap destroys every deployment whose TTL has run out.
//
// Note it walks `Reapable` itself rather than calling the cobra command:
// the command's job is flags, progress on stderr and an exit code, none
// of which mean anything here. The per-deployment work is the same
// function.
func (l *LiveActions) Reap(ctx context.Context) (api.ActionResult, error) {
	store := l.store()

	expired, unreadable, err := store.Reapable(time.Now())
	if err != nil {
		return api.ActionResult{}, err
	}

	var stages []StageSummary
	var failures []FailureSummary

	// Surfaced BEFORE any teardown, as the CLI does: an unreadable
	// record may describe running infrastructure this call is about to
	// report as handled.
	for _, readErr := range unreadable {
		failures = append(failures, FailureSummary{
			Layer: "live", Stage: "scan", Check: "readable", Command: "live reap",
			Detail: readErr.Error() + " — this record may describe running infrastructure that reap cannot reach",
		})
	}

	if len(expired) == 0 {
		stages = append(stages, StageSummary{
			Layer: "live", Stage: "reap", Status: StageStatusPass,
			Detail: "no expired deployments",
		})
	}

	for _, d := range expired {
		s, f := tearDownDeployment(ctx, l.runtime, store, d)
		stages = append(stages, s...)
		failures = append(failures, f...)
	}

	return actionResult(stages, failures), nil
}

// actionResult translates the CLI's staged output into the neutral shape
// the API seam speaks.
//
// `Clean` is the absence of failures and is reported as its own field,
// because "it worked" and "nothing complained" are different claims and
// ADR-0024 turns on the difference.
func actionResult(stages []StageSummary, failures []FailureSummary) api.ActionResult {
	out := api.ActionResult{
		Clean:    len(failures) == 0,
		Steps:    make([]api.ActionStep, 0, len(stages)),
		Failures: make([]api.ActionStep, 0, len(failures)),
	}
	for _, s := range stages {
		out.Steps = append(out.Steps, api.ActionStep{
			Stage: s.Stage, Status: string(s.Status), Detail: s.Detail,
		})
	}
	for _, f := range failures {
		out.Failures = append(out.Failures, api.ActionStep{
			Stage: f.Stage, Status: string(StageStatusFail), Detail: f.Detail,
		})
	}
	return out
}
