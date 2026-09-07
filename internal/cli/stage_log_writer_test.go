package cli

import (
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedLog struct {
	mu      sync.Mutex
	entries []LogEntry
}

func (r *recordedLog) capture() *AppLogger { return NewAppLogger(r) }

func (r *recordedLog) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line == "" {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal([]byte(line), &e); err == nil {
			r.entries = append(r.entries, e)
		}
	}
	return len(p), nil
}

func (r *recordedLog) snapshot() []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]LogEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

// The structured half of the tee.
//
// This comment used to justify the design with "the Live Run page renders
// LogEntry records, not raw text". It does not -- `live/+page.svelte`
// appends `JSON.stringify(msg)` for every frame. The entries are worth
// emitting because app.log is grep-able and the websocket carries them,
// not because the console parses them.
func TestStageProgressBecomesStructuredLogEntries(t *testing.T) {
	rec := &recordedLog{}
	w := newStageLogWriter(rec.capture(), io.Discard, LogEntry{Command: "test"})

	_, err := w.Write([]byte("  init: running\n  apply: done in 141s\n"))
	require.NoError(t, err)

	entries := rec.snapshot()
	require.Len(t, entries, 2)

	assert.Equal(t, "test", entries[0].Command)
	assert.Equal(t, "sandbox_deploy_progress", entries[0].Event)
	assert.Equal(t, "init", entries[0].Stage,
		"so app.log can be filtered by stage; the console does not group by it")
	assert.Equal(t, "init: running", entries[0].Detail)
	assert.Equal(t, "apply", entries[1].Stage)
}

// A caller using several Fprintf calls otherwise produces fragments, and
// a console appending fragments shows half a word.
func TestStageProgressEmitsWholeLinesNotWrites(t *testing.T) {
	rec := &recordedLog{}
	w := newStageLogWriter(rec.capture(), io.Discard, LogEntry{Command: "test"})

	_, _ = w.Write([]byte("  ini"))
	_, _ = w.Write([]byte("t: running\n"))

	entries := rec.snapshot()
	require.Len(t, entries, 1)
	assert.Equal(t, "init: running", entries[0].Detail)
}

func TestStageProgressFlushesATrailingLineOnClose(t *testing.T) {
	rec := &recordedLog{}
	w := newStageLogWriter(rec.capture(), io.Discard, LogEntry{Command: "test"})

	_, _ = w.Write([]byte("  apply: FAILED after 3s: transient provider error"))
	require.NoError(t, w.Close())

	entries := rec.snapshot()
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Detail, "transient provider error",
		"the reason a failed apply gives is the line that matters most")
}

// The READABLE half. `deploy` prints these lines and `test` must too.
//
// Sending stage progress only to the structured log made `infrafactory
// test` print a JSON object where `infrafactory deploy` prints
// `  apply: running` for the identical event -- and the S144 PR gate
// runs `test`, so the human reading the gate's job log got the worse
// rendering. The bytes here are the harness's own, unaltered.
func TestStageProgressStillPrintsTheReadableLine(t *testing.T) {
	rec := &recordedLog{}
	var text strings.Builder
	w := newStageLogWriter(rec.capture(), &text, LogEntry{Command: "test"})

	_, err := w.Write([]byte("  apply: running\n"))
	require.NoError(t, err)

	assert.Equal(t, "  apply: running\n", text.String(),
		"byte for byte what deploy writes, indent and all")
	require.Len(t, rec.snapshot(), 1, "and the structured entry as well")
}

// A failed apply is an error, like every other failure in the run log.
//
// Everything was `info` with no status, so `apply: FAILED` sat at the
// same level as `apply: running`. An operator grepping app.log for
// `"level":"error"` to find why a gate run failed missed the Layer 3
// apply failure -- the one line that says why.
func TestAFailedStageIsLoggedAsAnError(t *testing.T) {
	for name, tc := range map[string]struct {
		line  string
		level string
	}{
		"a failure":  {line: "  apply: FAILED after 141s: exit status 1\n", level: logLevelError},
		"giving up":  {line: "  apply: giving up after 2 attempt(s)\n", level: logLevelError},
		"an advance": {line: "  apply: running\n", level: logLevelInfo},
	} {
		t.Run(name, func(t *testing.T) {
			rec := &recordedLog{}
			w := newStageLogWriter(rec.capture(), io.Discard, LogEntry{Command: "test"})
			_, _ = w.Write([]byte(tc.line))

			entries := rec.snapshot()
			require.Len(t, entries, 1)
			assert.Equal(t, tc.level, entries[0].Level)
			if tc.level == logLevelError {
				assert.Equal(t, "failed", entries[0].Status,
					"and a status, so the failure is findable both ways")
			}
		})
	}
}

// The stage is the harness's token, or nothing -- never a guess.
//
// It was the text before the first colon of any line. A provider error
// rendered by `stageProgress.finished`'s `%v` is routinely multi-line, so
// `Error: creating instance` became stage "Error" and a continuation like
// `on main.tf line 12:` became stage "on main.tf line 12" -- garbage in a
// field a reader is meant to scan.
func TestTheStageIsNeverGuessedFromArbitraryText(t *testing.T) {
	rec := &recordedLog{}
	w := newStageLogWriter(rec.capture(), io.Discard, LogEntry{Command: "test"})

	_, _ = w.Write([]byte("  apply: FAILED after 3s: boom\n"))
	_, _ = w.Write([]byte("Error: creating instance\n"))
	_, _ = w.Write([]byte("  on main.tf line 12:\n"))

	entries := rec.snapshot()
	require.Len(t, entries, 3)
	assert.Equal(t, "apply", entries[0].Stage, "the harness's own shape")
	assert.Empty(t, entries[1].Stage, "not indented by the harness, so not a stage")
	assert.Empty(t, entries[2].Stage, "a sentence, not a token")

	// The LINE still gets through in every case. Refusing to name a
	// stage must never mean dropping the text.
	assert.Contains(t, entries[1].Detail, "creating instance")
	assert.Contains(t, entries[2].Detail, "main.tf line 12")
}

// A run stamps its scope, so two iterations are distinguishable.
//
// The command was hardcoded to "test" and RunID/Iteration never set, so
// under `infrafactory run` iteration 2's `apply: running` was
// byte-identical to iteration 1's in app.log -- and on the websocket,
// which is broadcast globally, a `test` running elsewhere injected
// indistinguishable lines into an unrelated run's console.
func TestStageProgressCarriesTheRunsScope(t *testing.T) {
	rec := &recordedLog{}
	w := newStageLogWriter(rec.capture(), io.Discard,
		LogEntry{Command: "run", RunID: "run-7", Iteration: 2})

	_, _ = w.Write([]byte("  apply: running\n"))

	entries := rec.snapshot()
	require.Len(t, entries, 1)
	assert.Equal(t, "run", entries[0].Command)
	assert.Equal(t, "run-7", entries[0].RunID)
	assert.Equal(t, 2, entries[0].Iteration)
}

// An empty command silently discards everything, so it is refused.
//
// `AppLogger.Log` drops any entry whose Command is empty -- no error, no
// panic, and `Write` still reports success. That is the same
// silent-discard failure this slice exists to fix, one layer down.
func TestAnEmptyCommandDoesNotSilentlyDiscard(t *testing.T) {
	rec := &recordedLog{}
	w := newStageLogWriter(rec.capture(), io.Discard, LogEntry{})

	_, _ = w.Write([]byte("  apply: running\n"))

	entries := rec.snapshot()
	require.Len(t, entries, 1, "the line must not vanish")
	assert.NotEmpty(t, entries[0].Command,
		"a writer with no command would drop every line it was given")
}
