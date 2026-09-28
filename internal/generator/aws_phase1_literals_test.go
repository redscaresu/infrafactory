package generator

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Each adapter builds its own PromptContext, so each is checked with the
// real AWS templates: one that forgets to copy Request.AMIID renders no id.
func TestAWSPromptsCarryTheAMIIDThroughBothAdapters(t *testing.T) {
	root := findRepoRoot(t)
	promptsDir := filepath.Join(root, "prompts")
	pitfallsDir := filepath.Join(root, "pitfalls")
	phases := []string{PhasePlanArchitecture, PhaseGenerateHCL}

	claude, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command: "claude", PromptsDir: promptsDir, PitfallsDir: pitfallsDir, Phases: phases,
	}, nil)
	require.NoError(t, err)
	openRouter, err := NewOpenRouterSeedGenerator(OpenRouterTransportConfig{
		APIKey: "test", Model: "test", BaseURL: "http://127.0.0.1", Timeout: time.Second,
		PromptsDir: promptsDir, PitfallsDir: pitfallsDir, Phases: phases,
	}, nil)
	require.NoError(t, err)

	awsInstance, err := os.ReadFile(filepath.Join(root, "scenarios", "training", "aws-instance.yaml"))
	require.NoError(t, err)
	req := Request{Cloud: "aws", ScenarioYAML: awsInstance, AMIID: "ami-0feedface0000001"}

	renderers := map[string]func(string) (string, error){
		"claude": func(phase string) (string, error) {
			return claude.renderPhasePrompt(phase, req, nil, nil, "")
		},
		"openrouter": func(phase string) (string, error) {
			return openRouter.renderPhasePrompt(phase, req, nil, nil, "")
		},
	}
	for name, render := range renderers {
		for _, phase := range phases {
			t.Run(name+"/"+phase, func(t *testing.T) {
				prompt, err := render(phase)
				require.NoError(t, err)
				assert.Contains(t, prompt, `ami = "ami-0feedface0000001"`)
			})
		}
	}
}

var (
	emptyYAMLBlockRe = regexp.MustCompile("```yaml\\s*```")
	sizeTableRowRe   = regexp.MustCompile("(?m)^\\| ([a-z]+) \\| ([a-z]+) \\| (.+) \\|$")
)

// ResolvedMappings is "" for every agent, so the AWS phase-1 prompt
// carries its size table literally. Every (resource, size) pair an AWS
// training scenario declares must have a row.
func TestAWSPhase1PromptCarriesALiteralSizeTable(t *testing.T) {
	root := findRepoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, "prompts", "aws", "phase1_plan_architecture.md"))
	require.NoError(t, err)
	prompt, err := RenderPromptTemplate(PhasePlanArchitecture, string(body), PromptContext{})
	require.NoError(t, err)

	// The scenario's own block is the one that may render empty here.
	assert.Len(t, emptyYAMLBlockRe.FindAllString(prompt, -1), 1, "an empty yaml block other than the scenario's")

	table := map[[2]string]string{}
	for _, row := range sizeTableRowRe.FindAllStringSubmatch(prompt, -1) {
		table[[2]string{row[1], row[2]}] = row[3]
	}

	scenarios, err := filepath.Glob(filepath.Join(root, "scenarios", "training", "aws-*.yaml"))
	require.NoError(t, err)
	declared := 0
	for _, path := range scenarios {
		payload, err := os.ReadFile(path)
		require.NoError(t, err)
		var sc struct {
			Resources map[string]struct {
				Size string `yaml:"size"`
			} `yaml:"resources"`
		}
		require.NoError(t, yaml.Unmarshal(payload, &sc), path)
		for kind, resource := range sc.Resources {
			if resource.Size == "" {
				continue
			}
			declared++
			assert.NotEmpty(t, table[[2]string{kind, resource.Size}], "%s declares %s %s, which has no row", filepath.Base(path), kind, resource.Size)
		}
	}
	require.NotZero(t, declared, "no AWS training scenario declares a size; the check is looking in the wrong place")
	assert.Contains(t, table[[2]string{"compute", "small"}], `"t3.micro"`)
}
