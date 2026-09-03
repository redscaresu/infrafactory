package cli

import (
	"encoding/json"
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

// The Live Run page renders LogEntry records, not raw text, so writing
// bytes at it would arrive as an unparsed blob beside properly formed
// events.
func TestStageProgressBecomesStructuredLogEntries(t *testing.T) {
	rec := &recordedLog{}
	w := newStageLogWriter(rec.capture(), "test")

	_, err := w.Write([]byte("  init: running\n  apply: done in 141s\n"))
	require.NoError(t, err)

	entries := rec.snapshot()
	require.Len(t, entries, 2)

	assert.Equal(t, "test", entries[0].Command)
	assert.Equal(t, "sandbox_deploy_progress", entries[0].Event)
	assert.Equal(t, "init", entries[0].Stage, "the console groups by stage")
	assert.Equal(t, "init: running", entries[0].Detail)
	assert.Equal(t, "apply", entries[1].Stage)
}

// A caller using several Fprintf calls otherwise produces fragments, and
// a console appending fragments shows half a word.
func TestStageProgressEmitsWholeLinesNotWrites(t *testing.T) {
	rec := &recordedLog{}
	w := newStageLogWriter(rec.capture(), "test")

	_, _ = w.Write([]byte("  ini"))
	_, _ = w.Write([]byte("t: running\n"))

	entries := rec.snapshot()
	require.Len(t, entries, 1)
	assert.Equal(t, "init: running", entries[0].Detail)
}

func TestStageProgressFlushesATrailingLineOnClose(t *testing.T) {
	rec := &recordedLog{}
	w := newStageLogWriter(rec.capture(), "test")

	_, _ = w.Write([]byte("  apply: FAILED after 3s: transient provider error"))
	require.NoError(t, w.Close())

	entries := rec.snapshot()
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Detail, "transient provider error",
		"the reason a failed apply gives is the line that matters most")
}

// A nil logger yields a nil writer, so the harness gets no writer rather
// than one that formats lines nobody will read.
func TestStageLogWriterIsNilWithoutALogger(t *testing.T) {
	w := newStageLogWriter(nil, "test")

	require.Nil(t, w)
	n, err := w.Write([]byte("anything\n"))
	require.NoError(t, err)
	assert.Equal(t, len("anything\n"), n)
	require.NoError(t, w.Close())
}

// A line with no stage prefix is still reported: silence is worse than
// an unlabelled line.
func TestStageProgressKeepsALineWithNoStage(t *testing.T) {
	rec := &recordedLog{}
	w := newStageLogWriter(rec.capture(), "test")

	_, _ = w.Write([]byte("something the harness said\n"))

	entries := rec.snapshot()
	require.Len(t, entries, 1)
	assert.Empty(t, entries[0].Stage)
	assert.Equal(t, "something the harness said", entries[0].Detail)
}
