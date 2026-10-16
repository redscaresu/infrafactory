package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const usageAvoidRule = "exit status 1 | stderr: ╷ Do NOT use attribute `enable_ipv6` on `scaleway_instance_server` — observed in scenario \"web\" to cause the failure above."

func writeUsagePitfalls(t *testing.T, dir, rule string) {
	t.Helper()
	yaml := "provider: scaleway\npitfalls:\n  - resource: scaleway_instance_server\n    rule: '" + rule + "'\n    source: avoid\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaleway.yaml"), []byte(yaml), 0o644))
}

func TestPitfallIDStableAcrossLoadsAndChangesWithRule(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeUsagePitfalls(t, dir, usageAvoidRule)

	first, err := LoadRenderedPitfalls(dir, "scaleway")
	require.NoError(t, err)
	second, err := LoadRenderedPitfalls(dir, "scaleway")
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Equal(t, PitfallID("scaleway", first[0]), PitfallID("scaleway", second[0]))

	writeUsagePitfalls(t, dir, usageAvoidRule+" Edited.")
	edited, err := LoadRenderedPitfalls(dir, "scaleway")
	require.NoError(t, err)
	require.Len(t, edited, 1)
	assert.NotEqual(t, PitfallID("scaleway", first[0]), PitfallID("scaleway", edited[0]))
}

func TestMatchBrokenPitfall(t *testing.T) {
	t.Parallel()
	rendered := []PitfallEntry{
		{Resource: "scaleway_instance_server", Rule: "Always set a `type`."},
		{Resource: "scaleway_instance_server", Rule: usageAvoidRule},
	}
	cases := map[string]struct {
		hcl  string
		want string
	}{
		"sets the avoided attribute": {`resource "scaleway_instance_server" "web" {
  enable_ipv6 = true
}`, PitfallID("scaleway", rendered[1])},
		"sets it on a single-line block": {`resource "scaleway_instance_server" "web" { enable_ipv6 = true }`, PitfallID("scaleway", rendered[1])},
		"sets it to a literal off value": {`resource "scaleway_instance_server" "web" {
  enable_ipv6 = false
}`, ""},
		"does not set it": {`resource "scaleway_instance_server" "web" {
  type = "DEV1-S"
}`, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(tc.hcl), 0o644))
			got, err := MatchBrokenPitfall(dir, "scaleway", "", "Error: with scaleway_instance_server.web, invalid", rendered)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPitfallsInPromptsKeepsOnlyShownEntries(t *testing.T) {
	t.Parallel()
	shown := PitfallEntry{Resource: "scaleway_instance_server", Rule: usageAvoidRule}
	unshown := PitfallEntry{Resource: "scaleway_block_volume", Rule: "Always set `size_in_gb`."}
	phases := []PhaseResult{
		{Name: PhasePlanArchitecture, Prompt: []byte("plan the stack")},
		{Name: PhaseGenerateHCL, Prompt: []byte("Pitfalls:\n" + renderPitfalls([]PitfallEntry{shown}))},
	}

	assert.Equal(t, []PitfallEntry{shown}, PitfallsInPrompts([]PitfallEntry{shown, unshown}, phases))
	assert.Empty(t, PitfallsInPrompts([]PitfallEntry{shown, unshown}, phases[:1]))
}
