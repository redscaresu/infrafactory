package generator

import (
	"bytes"
	"fmt"
	"os"
	"text/template"
)

type PromptContext struct {
	ScenarioYAML       string
	Constraints        string
	ResolvedMappings   string
	Overrides          string
	ArchitecturePlan   string
	AcceptanceCriteria string
	GeneratedFiles     string
	FeedbackJSON       string
	ProviderSchema     string
	Layer3Guidance     string
	Pitfalls           string
}

func RenderPromptTemplate(phase string, templateBody string, ctx PromptContext) (string, error) {
	tmpl, err := template.New(phase).Option("missingkey=error").Parse(templateBody)
	if err != nil {
		return "", NewGenerateError(ErrPromptRenderFailed, phase, fmt.Errorf("parse template: %w", err))
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, ctx); err != nil {
		return "", NewGenerateError(ErrPromptRenderFailed, phase, fmt.Errorf("execute template: %w", err))
	}

	return out.String(), nil
}

func RenderPromptFile(phase string, templatePath string, ctx PromptContext) (string, error) {
	payload, err := os.ReadFile(templatePath)
	if err != nil {
		return "", NewGenerateError(ErrPromptRenderFailed, phase, fmt.Errorf("read template %q: %w", templatePath, err))
	}

	return RenderPromptTemplate(phase, string(payload), ctx)
}
