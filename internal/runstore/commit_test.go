package runstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "t"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		require.NoError(t, cmd.Run(), "git %v", args)
	}
}

func gitCommitAll(t *testing.T, dir, msg string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", msg}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		require.NoError(t, cmd.Run(), "git %v", args)
	}
}

func TestCurrentCommitReportsTheRevisionOfACleanTree(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaleway.yaml"), []byte("pitfalls: []\n"), 0o600))
	gitCommitAll(t, dir, "seed")

	got := CurrentCommit(dir)
	assert.Len(t, got, 40, "a clean tree reports a bare 40-char sha: %q", got)
	assert.NotContains(t, got, "dirty")
}

// The dirty marker is the half that matters. A run generated from
// uncommitted pitfalls cannot be reproduced from the hash alone, and a
// bare hash would claim otherwise.
func TestCurrentCommitMarksAnUncommittedTreeDirty(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaleway.yaml"), []byte("pitfalls: []\n"), 0o600))
	gitCommitAll(t, dir, "seed")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaleway.yaml"), []byte("pitfalls: [changed]\n"), 0o600))

	got := CurrentCommit(dir)
	assert.True(t, strings.HasSuffix(got, "-dirty"), "an edited tree must say so: %q", got)
}

// Running outside a checkout is legitimate. Saying nothing is correct;
// inventing a value is not.
func TestCurrentCommitIsEmptyOutsideARepository(t *testing.T) {
	assert.Empty(t, CurrentCommit(t.TempDir()))
}
