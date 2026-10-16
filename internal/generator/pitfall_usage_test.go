package generator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const usageAvoidRule = "exit status 1 | stderr: ╷ Do NOT use attribute `enable_ipv6` on `scaleway_instance_server` — observed in scenario \"web\" to cause the failure above."

func writeUsagePitfalls(t *testing.T, dir, name, rule string) {
	t.Helper()
	yaml := "pitfalls:\n  - resource: scaleway_instance_server\n    rule: '" + rule + "'\n    source: avoid\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(yaml), 0o644))
}

func TestPitfallIDStableAcrossLoadsAndChangesWithRule(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeUsagePitfalls(t, dir, "scaleway.yaml", usageAvoidRule)

	first, err := LoadRenderedPitfalls(dir, "scaleway")
	require.NoError(t, err)
	second, err := LoadRenderedPitfalls(dir, "scaleway")
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Equal(t, PitfallID(first[0]), PitfallID(second[0]))

	writeUsagePitfalls(t, dir, "scaleway.yaml", usageAvoidRule+" Edited.")
	edited, err := LoadRenderedPitfalls(dir, "scaleway")
	require.NoError(t, err)
	require.Len(t, edited, 1)
	assert.NotEqual(t, PitfallID(first[0]), PitfallID(edited[0]))
}

func TestPitfallIDOfACommonEntryIsTheSameForEveryCloud(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeUsagePitfalls(t, dir, "scaleway.yaml", usageAvoidRule)
	writeUsagePitfalls(t, dir, "aws.yaml", usageAvoidRule)
	writeUsagePitfalls(t, dir, "common.yaml", "Never hardcode a secret.")

	scw, err := LoadRenderedPitfalls(dir, "scaleway")
	require.NoError(t, err)
	aws, err := LoadRenderedPitfalls(dir, "aws")
	require.NoError(t, err)
	require.Len(t, scw, 2)
	require.Len(t, aws, 2)

	assert.Equal(t, "scaleway.yaml", scw[0].File)
	assert.Equal(t, "common.yaml", scw[1].File)
	assert.Equal(t, PitfallID(scw[1]), PitfallID(aws[1]), "a common entry has one id across clouds")
	assert.NotEqual(t, PitfallID(scw[0]), PitfallID(aws[0]), "a cloud entry's id is scoped to its cloud")
}

func TestMatchBrokenPitfall(t *testing.T) {
	t.Parallel()
	rendered := []PitfallEntry{
		{Resource: "scaleway_instance_server", Rule: "Always set a `type`.", File: "scaleway.yaml"},
		{Resource: "scaleway_instance_server", Rule: usageAvoidRule, File: "scaleway.yaml"},
	}
	want := PitfallID(rendered[1])
	cases := map[string]struct {
		hcl, resource, detail, want string
	}{
		"the failing block sets the avoided attribute": {`resource "scaleway_instance_server" "web" {
  enable_ipv6 = true
}`, "", "Error: with scaleway_instance_server.web, invalid", want},
		"a single-line block":                 {`resource "scaleway_instance_server" "web" { enable_ipv6 = true }`, "", "Error: with scaleway_instance_server.web, invalid", want},
		"a module-qualified, indexed address": {`resource "scaleway_instance_server" "web" { enable_ipv6 = true }`, "module.net.scaleway_instance_server.web[0]", "", want},
		"a literal off value": {`resource "scaleway_instance_server" "web" {
  enable_ipv6 = false
}`, "", "Error: with scaleway_instance_server.web, invalid", ""},
		"a sibling block sets it, the failing one does not": {`resource "scaleway_instance_server" "a" {
  enable_ipv6 = true
}
resource "scaleway_instance_server" "b" {
  type = "DEV1-S"
}`, "scaleway_instance_server.b", "", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := MatchBrokenPitfall(map[string][]byte{"main.tf": []byte(tc.hcl)}, tc.resource, tc.detail, rendered)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// Each adapter reports the entries it rendered into each phase, from the
// real scaleway templates: phase 1 carries none, phase 2 carries them.
func TestAdaptersReportThePitfallsEachPhaseCarried(t *testing.T) {
	root := findRepoRoot(t)
	promptsDir := filepath.Join(root, "prompts")
	entries := []PitfallEntry{{Resource: "scaleway_instance_server", Rule: usageAvoidRule, File: "scaleway.yaml"}}
	claude, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command: "claude", PromptsDir: promptsDir, Phases: []string{PhasePlanArchitecture},
	}, nil)
	require.NoError(t, err)
	openRouter, err := NewOpenRouterSeedGenerator(OpenRouterTransportConfig{
		APIKey: "test", Model: "test", BaseURL: "http://127.0.0.1", Timeout: time.Second,
		PromptsDir: promptsDir, Phases: []string{PhasePlanArchitecture},
	}, nil)
	require.NoError(t, err)
	req := Request{Cloud: "scaleway", ScenarioYAML: []byte("scenario: web\n")}

	renderers := map[string]func(string) (string, []PitfallEntry, error){
		"claude": func(phase string) (string, []PitfallEntry, error) {
			return claude.renderPhasePrompt(phase, req, nil, nil, "", entries)
		},
		"openrouter": func(phase string) (string, []PitfallEntry, error) {
			return openRouter.renderPhasePrompt(phase, req, nil, nil, "", entries)
		},
	}
	for name, render := range renderers {
		t.Run(name, func(t *testing.T) {
			_, carried, err := render(PhasePlanArchitecture)
			require.NoError(t, err)
			assert.Empty(t, carried)
			prompt, carried, err := render(PhaseGenerateHCL)
			require.NoError(t, err)
			assert.Equal(t, entries, carried)
			assert.Contains(t, prompt, "enable_ipv6")
		})
	}
}

func TestRenderedPitfallsListsEachCarriedEntryOnce(t *testing.T) {
	t.Parallel()
	b := PitfallEntry{Resource: "scaleway_instance_server", Rule: "Do X. And Y.", File: "scaleway.yaml"}
	meta := GenerationMetadata{Phases: []PhaseResult{
		{Name: PhasePlanArchitecture},
		{Name: PhaseGenerateHCL, Pitfalls: []PitfallEntry{b}},
		{Name: PhaseSelfReview, Pitfalls: []PitfallEntry{b}},
	}}
	assert.Equal(t, []PitfallEntry{b}, meta.RenderedPitfalls())
}

func TestClaudeGenerateReportsCarriedPitfallsAndALoadError(t *testing.T) {
	t.Parallel()
	prompts := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(prompts, "phase1_plan_architecture.md"), []byte("S1 {{.ScenarioYAML}}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(prompts, "phase2_generate_hcl.md"), []byte("S2 {{.Pitfalls}}\n"), 0o644))
	generate := func(pitfallsDir string) *GeneratedCode {
		gen, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
			Command: "claude", PromptsDir: prompts, PitfallsDir: pitfallsDir,
			Phases: []string{PhasePlanArchitecture, PhaseGenerateHCL},
		}, &fakeClaudeRunner{outputs: []string{`{"region":"fr-par"}`, "# File: main.tf\nterraform {}"}})
		require.NoError(t, err)
		out, err := gen.Generate(context.Background(), Request{Cloud: "scaleway", ScenarioYAML: []byte("scenario: web")})
		require.NoError(t, err)
		return out
	}

	good := t.TempDir()
	writeUsagePitfalls(t, good, "scaleway.yaml", usageAvoidRule)
	entries, err := LoadRenderedPitfalls(good, "scaleway")
	require.NoError(t, err)
	out := generate(good)
	require.NoError(t, out.Metadata.PitfallsErr)
	require.Len(t, out.Metadata.Phases, 2)
	assert.Empty(t, out.Metadata.Phases[0].Pitfalls)
	assert.Equal(t, entries, out.Metadata.Phases[1].Pitfalls)

	bad := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bad, "scaleway.yaml"), []byte("pitfalls: [unclosed"), 0o644))
	out = generate(bad)
	assert.ErrorContains(t, out.Metadata.PitfallsErr, "scaleway.yaml")
	assert.Empty(t, out.Metadata.RenderedPitfalls())
}
