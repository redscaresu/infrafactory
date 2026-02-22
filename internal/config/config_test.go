package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidConfigAppliesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(filepath.Join("testdata", "valid.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Version != "1.0" {
		t.Fatalf("expected version 1.0, got %q", cfg.Version)
	}
	if cfg.Agent.Type != "claude-code" {
		t.Fatalf("expected agent.type claude-code, got %q", cfg.Agent.Type)
	}
	if cfg.Agent.MaxIterations != 5 {
		t.Fatalf("expected default max_iterations 5, got %d", cfg.Agent.MaxIterations)
	}
	if cfg.Agent.Claude.Command != "claude" {
		t.Fatalf("expected default agent.claude.command claude, got %q", cfg.Agent.Claude.Command)
	}
	if cfg.Agent.Claude.PhaseTimeoutSeconds != 300 {
		t.Fatalf("expected default agent.claude.phase_timeout_seconds 300, got %d", cfg.Agent.Claude.PhaseTimeoutSeconds)
	}
	if len(cfg.Agent.Phases) != 3 {
		t.Fatalf("expected default phases length 3, got %d", len(cfg.Agent.Phases))
	}
	if cfg.Mockway.AutoReset != true {
		t.Fatalf("expected default mockway.auto_reset true, got %v", cfg.Mockway.AutoReset)
	}
	if cfg.Paths.Output != "./output" {
		t.Fatalf("expected default paths.output ./output, got %q", cfg.Paths.Output)
	}
}

func TestLoadAgentTransportValidationFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		yaml             string
		expectedFieldSet []string
	}{
		{
			name: "negative phase delay",
			yaml: `version: "1.0"
agent:
  type: claude-code
  phase_delay_seconds: -1
mockway:
  url: http://localhost:8080
`,
			expectedFieldSet: []string{"agent.phase_delay_seconds"},
		},
		{
			name: "unknown and duplicate phase",
			yaml: `version: "1.0"
agent:
  type: claude-code
  phases:
    - plan_architecture
    - bad_phase
    - plan_architecture
mockway:
  url: http://localhost:8080
`,
			expectedFieldSet: []string{"agent.phases", "agent.phases[1]", "agent.phases[2]"},
		},
		{
			name: "openrouter missing required settings",
			yaml: `version: "1.0"
agent:
  type: openrouter
  openrouter:
    timeout_seconds: 0
    max_retries: -1
mockway:
  url: http://localhost:8080
`,
			expectedFieldSet: []string{
				"agent.openrouter.model",
				"agent.openrouter.timeout_seconds",
				"agent.openrouter.max_retries",
			},
		},
		{
			name: "claude command missing",
			yaml: `version: "1.0"
agent:
  type: claude-code
  claude:
    command: ""
mockway:
  url: http://localhost:8080
`,
			expectedFieldSet: []string{"agent.claude.command"},
		},
		{
			name: "claude phase timeout invalid",
			yaml: `version: "1.0"
agent:
  type: claude-code
  claude:
    command: claude
    phase_timeout_seconds: 0
mockway:
  url: http://localhost:8080
`,
			expectedFieldSet: []string{"agent.claude.phase_timeout_seconds"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "infrafactory.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			_, err := Load(path)
			if err == nil {
				t.Fatal("expected validation error")
			}

			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected *ValidationError, got %T (%v)", err, err)
			}

			for _, expectedField := range tc.expectedFieldSet {
				found := false
				for _, field := range validationErr.Fields {
					if field.Field == expectedField {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected validation error field %q, got %+v", expectedField, validationErr.Fields)
				}
			}
		})
	}
}

func TestLoadOpenRouterConfigDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "infrafactory.yaml")
	content := strings.Join([]string{
		`version: "1.0"`,
		`agent:`,
		`  type: openrouter`,
		`  openrouter:`,
		`    model: anthropic/claude-3.5-sonnet`,
		`mockway:`,
		`  url: http://localhost:8080`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Agent.OpenRouter.Model != "anthropic/claude-3.5-sonnet" {
		t.Fatalf("expected configured model, got %q", cfg.Agent.OpenRouter.Model)
	}
	if cfg.Agent.OpenRouter.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("expected default base url, got %q", cfg.Agent.OpenRouter.BaseURL)
	}
	if cfg.Agent.OpenRouter.TimeoutSeconds != 60 {
		t.Fatalf("expected default timeout 60, got %d", cfg.Agent.OpenRouter.TimeoutSeconds)
	}
	if cfg.Agent.OpenRouter.MaxRetries != 2 {
		t.Fatalf("expected default max retries 2, got %d", cfg.Agent.OpenRouter.MaxRetries)
	}
}

func TestLoadMissingRequiredFieldsReturnsTypedValidationError(t *testing.T) {
	t.Parallel()

	_, err := Load(filepath.Join("testdata", "missing-required.yaml"))
	if err == nil {
		t.Fatal("expected validation error")
	}

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *ValidationError, got %T (%v)", err, err)
	}

	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected errors.Is(..., ErrInvalidConfig) to be true, got %v", err)
	}

	if len(validationErr.Fields) != 3 {
		t.Fatalf("expected 3 field errors, got %d", len(validationErr.Fields))
	}

	expected := map[string]bool{
		"agent.type":  false,
		"mockway.url": false,
		"version":     false,
	}

	for _, field := range validationErr.Fields {
		if _, ok := expected[field.Field]; ok {
			expected[field.Field] = true
		}
	}

	for field, seen := range expected {
		if !seen {
			t.Fatalf("expected validation error for field %q", field)
		}
	}
}
