package generator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Phase 2 never sees the scenario YAML, so the user-data instruction reaches
// the model only through Request.UserDataLine. Each adapter builds its own
// PromptContext, so each is checked: one that forgets to copy the field
// renders no line.
func TestPhase2PromptCarriesUserDataLineOnlyWhenSet(t *testing.T) {
	root := findRepoRoot(t)
	promptsDir := filepath.Join(root, "prompts")
	pitfallsDir := filepath.Join(root, "pitfalls")
	phases := []string{PhaseGenerateHCL}

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

	renderers := map[string]func(Request) (string, error){
		"claude": func(req Request) (string, error) {
			return claude.renderPhasePrompt(PhaseGenerateHCL, req, nil, nil, "")
		},
		"openrouter": func(req Request) (string, error) {
			return openRouter.renderPhasePrompt(PhaseGenerateHCL, req, nil, nil, "")
		},
	}
	for name, render := range renderers {
		t.Run(name, func(t *testing.T) {
			noService := Request{Cloud: "aws", ScenarioYAML: awsInstance}
			prompt, err := render(noService)
			require.NoError(t, err)
			assert.NotContains(t, prompt, AWSUserDataLine)
			assert.NotContains(t, prompt, AWSUserDataFile)

			withService := noService
			withService.UserDataLine = AWSUserDataLine
			prompt, err = render(withService)
			require.NoError(t, err)
			assert.Contains(t, prompt, AWSUserDataLine)
		})
	}
}
