package harness

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingWriter captures progress AND when each line arrived relative
// to the stages, which is the property that matters.
type recordingWriter struct {
	mu    sync.Mutex
	lines []string
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line != "" {
			w.lines = append(w.lines, strings.TrimSpace(line))
		}
	}
	return len(p), nil
}

func (w *recordingWriter) snapshot() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, len(w.lines))
	copy(out, w.lines)
	return out
}

// observingRunner lets a test inspect progress from INSIDE a stage.
type observingRunner struct {
	duringApply []string
	writer      *recordingWriter
	fail        error
	failFirst   bool
	calls       int
}

func (r *observingRunner) Run(_ context.Context, cmd Command) (CommandResult, error) {
	r.calls++
	if len(cmd.Args) > 0 && cmd.Args[0] == "apply" {
		// What a watcher could see while the apply is still running.
		r.duringApply = r.writer.snapshot()
		if r.failFirst && r.calls <= 3 {
			return CommandResult{}, r.fail
		}
	}
	return CommandResult{Stdout: []byte("ok")}, nil
}

// The defect this whole slice exists to fix, pinned properly.
//
// The first version of S163 tee'd the deploy command's stderr, which is
// written twice BEFORE any cloud work and once AFTER it. `CommandRunner`
// returns a fully buffered result, so `tofu`'s own output does not exist
// until each process exits -- meaning the apply produced no progress at
// all, and the tests could not tell because their fixtures were written
// in one synchronous call.
//
// This asserts what a watcher can see WHILE the apply is still running,
// which a buffer-then-dump implementation cannot satisfy.
func TestProgressIsVisibleWhileTheApplyIsStillRunning(t *testing.T) {
	writer := &recordingWriter{}
	runner := &observingRunner{writer: writer}

	_, err := NewSandboxDeployHarness(runner).Run(
		context.Background(), t.TempDir(), map[string]string{}, writer)
	require.NoError(t, err)

	seen := strings.Join(runner.duringApply, "\n")
	assert.Contains(t, seen, "init: running",
		"a watcher must know init happened before the apply finishes")
	assert.Contains(t, seen, "plan: done",
		"and that plan completed")
	assert.Contains(t, seen, "apply: running",
		"and that the apply is what is taking the time")
}

// A silent retry is indistinguishable from a stage that is simply slow,
// and a Layer 3 apply really does retry after a transient provider
// error.
func TestAnApplyRetryIsReported(t *testing.T) {
	writer := &recordingWriter{}
	runner := &observingRunner{writer: writer, fail: assertProviderError{}, failFirst: true}

	_, _ = NewSandboxDeployHarness(runner).Run(
		context.Background(), t.TempDir(), map[string]string{}, writer)

	assert.Contains(t, strings.Join(writer.snapshot(), "\n"), "retrying",
		"a frozen log during a silent retry reads as hung")
}

type assertProviderError struct{}

func (assertProviderError) Error() string { return "transient provider error" }

// A caller with nowhere to send progress is the ordinary case and must
// cost nothing.
func TestSandboxDeployAcceptsANilProgressWriter(t *testing.T) {
	runner := &observingRunner{writer: &recordingWriter{}}

	_, err := NewSandboxDeployHarness(runner).Run(
		context.Background(), t.TempDir(), map[string]string{}, nil)

	require.NoError(t, err)
}

// Every stage the harness runs must announce itself, so a stage added
// later cannot be silently invisible to a watcher.
func TestEveryStageReportsItself(t *testing.T) {
	writer := &recordingWriter{}
	runner := &observingRunner{writer: writer}

	_, err := NewSandboxDeployHarness(runner).Run(
		context.Background(), t.TempDir(), map[string]string{}, writer)
	require.NoError(t, err)

	joined := strings.Join(writer.snapshot(), "\n")
	for _, stage := range []string{"init", "plan", "apply"} {
		assert.Contains(t, joined, stage+": running", "%s must announce itself", stage)
		assert.Contains(t, joined, stage+": done", "%s must report completion", stage)
	}
}

// A stage that FAILED must not be reported as done.
//
// An earlier version called `done` unconditionally, before inspecting
// the error, so the last line a watcher saw on a failed deploy was
// "init: done in 2s" followed by silence -- a failure rendered as
// completion, and a stream that stops without saying why.
//
// TestEveryStageReportsItself actively pinned that wrong behaviour,
// which is why it is not enough on its own.
func TestAFailedStageIsNotReportedAsDone(t *testing.T) {
	for _, failing := range []string{"init", "plan", "apply"} {
		t.Run(failing, func(t *testing.T) {
			writer := &recordingWriter{}
			runner := &stageFailingRunner{failStage: failing}

			_, err := NewSandboxDeployHarness(runner).Run(
				context.Background(), t.TempDir(), map[string]string{}, writer)
			require.Error(t, err)

			joined := strings.Join(writer.snapshot(), "\n")
			assert.Contains(t, joined, failing+": FAILED",
				"the stage that failed must say so")
			assert.NotContains(t, joined, failing+": done",
				"a failure must never be rendered as completion")
		})
	}
}

// stageFailingRunner fails one named stage.
type stageFailingRunner struct {
	failStage string
}

func (r *stageFailingRunner) Run(_ context.Context, cmd Command) (CommandResult, error) {
	if len(cmd.Args) > 0 && cmd.Args[0] == r.failStage {
		return CommandResult{Stderr: []byte("boom")}, assertProviderError{}
	}
	return CommandResult{Stdout: []byte("ok")}, nil
}
