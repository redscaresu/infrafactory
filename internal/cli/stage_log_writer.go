package cli

import (
	"bytes"
	"strings"
	"sync"
)

// stageLogWriter turns the harness's stage progress into log entries.
//
// # Why an adapter and not another sink
//
// S163 gave `deploy` live stage progress by handing the harness an
// `io.Writer` that reached the websocket. `run` and `test` got `nil`,
// so their Layer 3 apply stayed silent for minutes -- on the **Live Run
// page**, which is the screen somebody actually watches while a PR gate
// runs, and the one a demo is pointed at.
//
// That page renders `LogEntry` records, not raw text: the run console is
// built from the structured log, and writing bytes at it would arrive as
// an unparsed blob beside properly formed events. So the harness's
// writer is adapted into the log rather than the log being bypassed.
//
// Line-buffered for the same reason `ProgressSink` is: a caller using
// several Fprintf calls otherwise produces fragments, and a console
// appending fragments shows half a word.
type stageLogWriter struct {
	log     func(LogEntry)
	command string

	mu      sync.Mutex
	partial bytes.Buffer
}

// newStageLogWriter adapts a logger. A nil logger yields nil, so the
// harness receives no writer at all rather than one that discards --
// there is nothing to gain from formatting lines nobody will read.
func newStageLogWriter(logger *AppLogger, command string) *stageLogWriter {
	if logger == nil {
		return nil
	}
	return &stageLogWriter{log: logger.Log, command: command}
}

func (w *stageLogWriter) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
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
	if w == nil {
		return nil
	}
	w.mu.Lock()
	tail := w.partial.String()
	w.partial.Reset()
	w.mu.Unlock()

	w.emit(tail)
	return nil
}

// emit records one line.
//
// The harness writes `  <stage>: <what happened>`, so the stage is
// recovered into its own field: the console groups by it, and a reader
// scanning a long apply looks for "which stage" before they read the
// words.
func (w *stageLogWriter) emit(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	stage := ""
	if idx := strings.Index(trimmed, ":"); idx > 0 {
		stage = strings.TrimSpace(trimmed[:idx])
	}

	w.log(LogEntry{
		Level:   logLevelInfo,
		Command: w.command,
		Event:   "sandbox_deploy_progress",
		Stage:   stage,
		Detail:  trimmed,
	})
}
