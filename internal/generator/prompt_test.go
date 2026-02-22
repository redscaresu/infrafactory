package generator

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderPromptTemplate(t *testing.T) {
	t.Parallel()

	templateBody := `SCENARIO:
{{.ScenarioYAML}}
{{if .FeedbackJSON}}FEEDBACK:
{{.FeedbackJSON}}{{end}}
`

	cases := []struct {
		name           string
		ctx            PromptContext
		contains       []string
		doesNotContain []string
	}{
		{
			name: "without feedback",
			ctx: PromptContext{
				ScenarioYAML: "scenario: web-app-paris",
			},
			contains:       []string{"SCENARIO:", "scenario: web-app-paris"},
			doesNotContain: []string{"FEEDBACK:"},
		},
		{
			name: "with feedback",
			ctx: PromptContext{
				ScenarioYAML: "scenario: web-app-paris",
				FeedbackJSON: `{"failures":[{"check":"policy"}]}`,
			},
			contains: []string{"SCENARIO:", "FEEDBACK:", `{"failures":[{"check":"policy"}]}`},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rendered, err := RenderPromptTemplate("phase2", templateBody, tc.ctx)
			if err != nil {
				t.Fatalf("render template: %v", err)
			}

			for _, expected := range tc.contains {
				if !strings.Contains(rendered, expected) {
					t.Fatalf("expected rendered prompt to contain %q, got:\n%s", expected, rendered)
				}
			}
			for _, unexpected := range tc.doesNotContain {
				if strings.Contains(rendered, unexpected) {
					t.Fatalf("expected rendered prompt not to contain %q, got:\n%s", unexpected, rendered)
				}
			}
		})
	}
}

func TestRenderPromptFile(t *testing.T) {
	t.Parallel()

	rendered, err := RenderPromptFile("phase1", filepath.Join("testdata", "prompt.tmpl"), PromptContext{
		ScenarioYAML:       "scenario: test",
		Constraints:        "region=fr-par",
		AcceptanceCriteria: "type=policy",
	})
	if err != nil {
		t.Fatalf("render prompt file: %v", err)
	}

	if !strings.Contains(rendered, "scenario: test") || !strings.Contains(rendered, "region=fr-par") {
		t.Fatalf("unexpected rendered prompt:\n%s", rendered)
	}
}

func TestRenderPromptTemplateMissingKeyReturnsTypedError(t *testing.T) {
	t.Parallel()

	_, err := RenderPromptTemplate("phase1", `{{.UnknownField}}`, PromptContext{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrPromptRenderFailed) {
		t.Fatalf("expected ErrPromptRenderFailed, got %v", err)
	}
	if !errors.Is(err, ErrGenerateFailed) {
		t.Fatalf("expected ErrGenerateFailed, got %v", err)
	}
}
