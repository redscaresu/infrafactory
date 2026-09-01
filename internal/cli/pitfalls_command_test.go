package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
)

// retireRuntime points the corpus at a temp dir so a test cannot retire
// entries from the repository's real pitfalls files.
func retireRuntime(t *testing.T, entries ...generator.PitfallEntry) (*CommandRuntime, string) {
	t.Helper()
	dir := t.TempDir()
	payload, err := yaml.Marshal(generator.PitfallsFile{Provider: "scaleway", Pitfalls: entries})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaleway.yaml"), payload, 0o644))

	return &CommandRuntime{Config: config.Config{Paths: config.PathsConfig{Pitfalls: dir}}}, dir
}

func runRetire(t *testing.T, rt *CommandRuntime, out *strings.Builder, args ...string) error {
	t.Helper()
	cmd := &cobra.Command{Use: "retire"}
	cmd.Flags().Duration("older-than", generator.DefaultLiveRetention, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	require.NoError(t, cmd.ParseFlags(args))
	cmd.SetOut(out)
	cmd.SetErr(out)
	return runPitfallsRetireCommand(cmd, []string{"scaleway"}, rt)
}

func staleLive(resource string, age time.Duration) generator.PitfallEntry {
	return generator.PitfallEntry{
		Resource: resource, Rule: "a rule about " + resource, Source: generator.LiveSource,
		LastSeen: time.Now().Add(-age).UTC().Format(time.RFC3339),
	}
}

// A corpus that quietly drops entries is indistinguishable from one that
// never learned them -- the rule the D6 purge established.
func TestRetireNamesEveryEntryItRemoved(t *testing.T) {
	rt, _ := retireRuntime(t, staleLive("scaleway_lb", 30*24*time.Hour))

	var out strings.Builder
	require.NoError(t, runRetire(t, rt, &out, "--older-than", "336h"))

	assert.Contains(t, out.String(), "retired 1 live pitfall(s)")
	assert.Contains(t, out.String(), "scaleway_lb")
	assert.Contains(t, out.String(), "last seen")
	assert.Contains(t, out.String(), "a rule about scaleway_lb", "the rule itself, not just a count")
}

// Retirement deletes learning, so an operator can see what a threshold
// would take out before it takes it out.
func TestRetireDryRunChangesNothing(t *testing.T) {
	rt, dir := retireRuntime(t, staleLive("scaleway_lb", 30*24*time.Hour))

	var out strings.Builder
	require.NoError(t, runRetire(t, rt, &out, "--older-than", "336h", "--dry-run"))

	assert.Contains(t, out.String(), "--dry-run")
	assert.Contains(t, out.String(), "would be retired")

	payload, err := os.ReadFile(filepath.Join(dir, "scaleway.yaml"))
	require.NoError(t, err)
	var pf generator.PitfallsFile
	require.NoError(t, yaml.Unmarshal(payload, &pf))
	assert.Len(t, pf.Pitfalls, 1, "a dry run must not remove anything")
}

func TestRetireSaysSoWhenNothingIsStale(t *testing.T) {
	rt, _ := retireRuntime(t, staleLive("scaleway_lb", time.Hour))

	var out strings.Builder
	require.NoError(t, runRetire(t, rt, &out, "--older-than", "336h"))

	assert.Contains(t, out.String(), "no live pitfalls in scaleway have gone stale")
}

// A mistyped threshold would otherwise delete everything learned from
// running services in one command.
func TestRetireRefusesANonPositiveThreshold(t *testing.T) {
	rt, dir := retireRuntime(t, staleLive("scaleway_lb", 30*24*time.Hour))

	for _, flag := range []string{"0s", "-1h"} {
		err := runRetire(t, rt, &strings.Builder{}, "--older-than", flag)
		require.Error(t, err, "--older-than %s", flag)
		assert.Contains(t, err.Error(), "every live pitfall would be retired")
	}

	payload, err := os.ReadFile(filepath.Join(dir, "scaleway.yaml"))
	require.NoError(t, err)
	var pf generator.PitfallsFile
	require.NoError(t, yaml.Unmarshal(payload, &pf))
	assert.Len(t, pf.Pitfalls, 1)
}

// The dry-run must refuse exactly what the real run refuses, or it
// teaches the operator the wrong thing about what is safe to type.
func TestRetireDryRunRefusesTheSameThresholds(t *testing.T) {
	rt, _ := retireRuntime(t, staleLive("scaleway_lb", 30*24*time.Hour))

	err := runRetire(t, rt, &strings.Builder{}, "--older-than", "0s", "--dry-run")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "every live pitfall would be retired")
}

// Corpus maintenance must not depend on the LLM being configured.
// buildRuntime constructs the Claude transport by default and that
// construction can fail -- a missing binary, an unreadable prompts
// directory -- which would make housekeeping impossible on exactly the
// machine doing housekeeping rather than generation.
func TestRetireDoesNotRequireAGenerator(t *testing.T) {
	h := newCommandTestHarness(t)
	require.NoError(t, os.MkdirAll(h.PitfallsDir(), 0o755))
	payload, err := yaml.Marshal(generator.PitfallsFile{Provider: "scaleway"})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(h.PitfallsDir(), "scaleway.yaml"), payload, 0o644))

	// The agent points at a binary that does not exist, which is what
	// makes this a test rather than a tautology: buildRuntime would
	// otherwise construct a working transport and prove nothing.
	raw, err := os.ReadFile(h.ConfigPath)
	require.NoError(t, err)
	patched := strings.Replace(string(raw), "type: claude-code",
		"type: claude-code\n  claude:\n    command: /nonexistent/claude", 1)
	require.NotEqual(t, string(raw), patched, "the harness config must declare a claude-code agent")
	require.NoError(t, os.WriteFile(h.ConfigPath, []byte(patched), 0o644))

	cmd := &cobra.Command{Use: "retire"}
	cmd.Flags().Duration("older-than", generator.DefaultLiveRetention, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("output", string(OutputModeHuman), "")
	cmd.Flags().String("config", h.ConfigPath, "")
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	handler := (&rootConfig{}).withRuntimeNoGenerator("pitfalls retire", runPitfallsRetireCommand)

	assert.NoError(t, handler(cmd, []string{"scaleway"}),
		"corpus maintenance must not fail because the agent is unconfigured")
}
