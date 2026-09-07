package cli

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// stageLogWriter tees the harness's stage progress: readable text for
// whoever is running the command, structured entries for the run console.
//
// # Why a tee, and not the adapter this started as
//
// S163 gave `deploy` live stage progress by handing the harness an
// `io.Writer` that reached the websocket. `run` and `test` got `nil`, so
// their Layer 3 apply stayed silent for minutes -- including on the
// **Live Run page**, which is the screen somebody watches while a PR gate
// runs.
//
// The first version of this type sent the lines ONLY to the structured
// log, justified by the claim that the Live Run console renders
// `LogEntry` records and groups by stage, so raw bytes would arrive as an
// unparsed blob. **That claim was false in both halves**, and review
// caught it:
//
//   - `live/+page.svelte` appends `JSON.stringify(msg)` for every frame,
//     so the console renders a blob for EVERYTHING. There was no
//     well-formed rendering to be inconsistent with.
//   - `deriveCurrentStage` matches only `event == "stage_start"`, so the
//     recovered stage fed no grouping, no badge and no filter.
//
// Worse, it made the common case worse. `AppLogger`'s default sink is
// stderr, so `infrafactory test` printed a JSON object where
// `infrafactory deploy` prints `  apply: running` for the identical
// event -- and the S144 PR gate runs `test`, so the human reading the
// gate's job log got the degraded rendering. That is the opposite of
// what this slice is for.
//
// So: the readable line goes where `deploy` sends it, and the structured
// entry goes to the log as well. Neither audience is traded for the
// other.
//
// Line-buffered for the same reason `ProgressSink` is: a caller using
// several Fprintf calls otherwise produces fragments, and a console
// appending fragments shows half a word.
type stageLogWriter struct {
	log   func(LogEntry)
	text  io.Writer
	entry LogEntry

	mu      sync.Mutex
	partial bytes.Buffer
	closed  bool
}

// newStageLogWriter builds a writer that always works.
//
// It used to return a nil `*stageLogWriter` when there was no logger,
// which callers then had to keep out of an `io.Writer` by hand: a nil
// one of those becomes a NON-nil interface, `stageProgress`'s
// `p.out != nil` guard passes, and every stage line is formatted and
// dropped. That hazard was defended at the call site with an interface
// dance, an ADR paragraph and two tests -- to protect a branch that
// cannot be reached, because `CommandRuntime` always builds a logger.
//
// Removing the branch removes the trap. `AppLogger.Log` already no-ops
// on a nil receiver, and binding `logger.Log` on a nil `*AppLogger` is
// legal, so there is nothing left to guard against.
func newStageLogWriter(logger *AppLogger, text io.Writer, entry LogEntry) *stageLogWriter {
	// An empty Command SILENTLY DISCARDS every line.
	//
	// `AppLogger.Log` returns early on `entry.Command == ""` -- no
	// error, no panic -- while `Write` still reports success. That is
	// the same invisible-drop failure this slice exists to fix, one
	// layer down, and a caller that constructs the writer wrong would
	// have got a healthy-looking one that threw everything away.
	//
	// "unknown" rather than a guess at the real command: the line
	// survives, and the anomaly is visible in the log instead of being
	// papered over as `test`.
	if entry.Command == "" {
		entry.Command = "unknown"
	}
	return &stageLogWriter{log: logger.Log, text: text, entry: entry}
}

func (w *stageLogWriter) Write(p []byte) (int, error) {
	// The readable half FIRST, and unbuffered.
	//
	// This is the byte-for-byte stream `deploy` produces, so the CLI and
	// the PR gate read exactly what they did before this type existed.
	// Buffering it into lines would be a second place for the trailing
	// line to go missing.
	if w.text != nil {
		_, _ = w.text.Write(p)
	}

	w.mu.Lock()
	w.partial.Write(p)

	var lines []string
	for {
		buffered := w.partial.Bytes()
		idx := bytes.IndexByte(buffered, '\n')
		if idx < 0 {
			break
		}
		lines = append(lines, string(bytes.TrimRight(buffered[:idx], "\r")))
		w.partial.Next(idx + 1)
	}
	w.mu.Unlock()

	for _, line := range lines {
		w.emit(line)
	}
	return len(p), nil
}

// Close flushes a trailing line with no newline.
func (w *stageLogWriter) Close() error {
	w.mu.Lock()
	tail := w.partial.String()
	w.partial.Reset()
	// Recorded so a test at the CALL SITE can assert Close was reached.
	// What Close DOES is covered separately; that the call site makes it
	// is a different claim, and removing the call left the whole package
	// green.
	w.closed = true
	w.mu.Unlock()

	w.emit(tail)
	return nil
}

// stageOf recovers the stage from a line the harness wrote.
//
// The harness writes `  <stage>: <text>` and nothing else, so the shape
// is checked rather than assumed: an indent, then a single token, then a
// colon. Taking the first colon of arbitrary text instead turned
// `Error: creating instance` into stage "Error" and a continuation line
// like `on main.tf line 12:` into stage "on main.tf line 12" -- garbage
// in a field a reader is meant to scan. A provider error rendered by
// `stageProgress.finished`'s `%v` is frequently multi-line, so those are
// ordinary lines, not exotic ones.
//
// An empty return means "this line does not name a stage", which is a
// better answer than a wrong one.
func stageOf(raw, trimmed string) string {
	if !strings.HasPrefix(raw, "  ") {
		return ""
	}
	idx := strings.Index(trimmed, ":")
	if idx <= 0 {
		return ""
	}
	stage := trimmed[:idx]
	if strings.ContainsAny(stage, " \t") {
		return ""
	}
	return stage
}

// emit records one line.
func (w *stageLogWriter) emit(raw string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return
	}

	entry := w.entry
	entry.Event = "sandbox_deploy_progress"
	entry.Stage = stageOf(raw, trimmed)
	entry.Detail = trimmed

	// A failed apply is an ERROR, like every other failure in the run
	// log.
	//
	// Everything here was `info` with no status, so `  apply: FAILED
	// after 141s: exit status 1` was indistinguishable at the level from
	// `  apply: running`. `run_command.go` uses error+failed at
	// seventeen sites for exactly this class, and an operator grepping
	// app.log for `"level":"error"` to find why a gate run failed would
	// have missed the Layer 3 apply failure entirely -- the one line
	// that says why.
	switch {
	case strings.Contains(trimmed, ": FAILED"), strings.Contains(trimmed, ": giving up"):
		entry.Level = logLevelError
		entry.Status = "failed"
	default:
		entry.Level = logLevelInfo
	}

	w.log(entry)
}
