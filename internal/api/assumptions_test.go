package api

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Assumptions this package's COMMENTS depend on.
//
// # Why these are tests and not sentences
//
// Every serious defect found in this package over the week of 2026-09-01
// was a confident claim about how some OTHER component behaves, written
// into a comment and never checked:
//
//   - "the YAML error names the file it was read from" -- it does not,
//     and acting on that stripped a line number from the page that exists
//     to fix the file
//   - "the cause is on the command's stderr" -- it is not, and acting on
//     that destroyed the most useful errors on the deploy path
//   - "there is no spelling that does not call .Error()" -- `%v` is one,
//     and the audit built on that claim had a hole for two rounds
//
// None of the code was wrong. The REASONS were, and a reason written as
// a confident paragraph is more durable than a terse one: it survives
// review, because it reads as something somebody already checked.
//
// So the rule this file exists to enforce is:
//
//	FACTS GO IN TESTS. DECISIONS GO IN PROSE.
//
// A claim about how a dependency behaves is a fact and belongs here,
// cited by name from the comment that relies on it. A trade-off ("we
// chose a tee over an adapter", "the exit code stays 0 because forget is
// deliberate") cannot be tested and stays prose.
//
// The point is not diligence. It is that writing the test IS the check,
// so there is no gap left between meaning to verify and verifying. And
// unlike a comment, it fails when the dependency changes underneath it.

// yaml.Unmarshal receives BYTES and has no filename to report.
//
// Relied on by: handlers_pitfalls.go, which shows the parse error to the
// reader. I once withheld it believing the opposite.
func TestAssumption_YAMLErrorsCannotNameTheFile(t *testing.T) {
	var v map[string]any
	err := yaml.Unmarshal([]byte("a: 1\n b: 2\n"), &v)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 2",
		"the line number is the useful part, and the reason it is shown")
	assert.NotContains(t, err.Error(), ".yaml",
		"no filename: Unmarshal never saw one")
	assert.NotContains(t, err.Error(), string(filepath.Separator),
		"and therefore no path")
}

// A filesystem error carries an ABSOLUTE PATH in its text.
//
// Relied on by: every writeInternalError call site, and the
// error_body_audit rule. If this ever stopped being true the audit would
// still be harmless, but its justification would be gone.
func TestAssumption_FilesystemErrorsCarryTheirPath(t *testing.T) {
	_, err := os.ReadFile(filepath.Join(t.TempDir(), "no-such-file"))

	require.Error(t, err)
	var pathErr *fs.PathError
	require.True(t, errors.As(err, &pathErr), "an *fs.PathError, as assumed")
	assert.Contains(t, err.Error(), "no-such-file",
		"the path is IN the message, which is why bodies may not carry it")
}

// livestore.Get wraps its cause, so the store's path reaches the caller.
//
// Relied on by: deploymentsHandler's `unreadable` handling, which logs
// the error and sends a composed sentence instead.
func TestAssumption_LiveStoreGetWrapsThePathIntoItsError(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "dep-x.json")
	require.NoError(t, os.WriteFile(f, []byte(`{"id":"dep-x"}`), 0o600))
	require.NoError(t, os.Chmod(f, 0o000))
	t.Cleanup(func() { _ = os.Chmod(f, 0o600) })
	if _, probe := os.ReadFile(f); probe == nil {
		t.Skip("this filesystem does not enforce the mode bits")
	}

	_, err := livestore.NewFilesystemStore(root).Get("dep-x")

	require.Error(t, err)
	assert.Contains(t, err.Error(), root,
		"the store's absolute path is in the text a naive handler would echo")
}
