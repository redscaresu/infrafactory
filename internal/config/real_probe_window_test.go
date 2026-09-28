package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRealProbeWindowPinnedToDefault pins the repo's infrafactory.yaml
// real_probes window to the config.Default() value, and checks the
// nearby comments don't still describe the old 24-attempt window.
// Both files agreed on the operative retries value (60) while their
// comments drifted to "24 attempts" -- this test fails if either the
// values diverge or a comment regresses.
func TestRealProbeWindowPinnedToDefault(t *testing.T) {
	root := repoRoot(t)

	cfg, err := Load(filepath.Join(root, "infrafactory.yaml"))
	require.NoError(t, err)

	want := Default().Validation.RealProbes
	assert.Equal(t, want, cfg.Validation.RealProbes)

	for _, rel := range []string{"infrafactory.yaml", "internal/config/config.go"} {
		content, err := os.ReadFile(filepath.Join(root, rel))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "24 attempts", "%s", rel)
		assert.NotContains(t, string(content), "24 x (5s", "%s", rel)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}
