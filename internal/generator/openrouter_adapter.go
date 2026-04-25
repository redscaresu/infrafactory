package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type OpenRouterTransportConfig struct {
	APIKey           string
	Model            string
	BaseURL          string
	Timeout          time.Duration
	MaxRetries       int
	RetryDelay       time.Duration
	PhaseDelay       time.Duration
	PromptsDir       string
	PitfallsDir      string
	Phases           []string
	Constraints      string
	ResolvedMappings string
	Overrides        string
	Acceptance       string
}

type OpenRouterSeedGenerator struct {
	cfg        OpenRouterTransportConfig
	httpClient *http.Client
	sleep      func(time.Duration)
}

func NewOpenRouterSeedGenerator(cfg OpenRouterTransportConfig, httpClient *http.Client) (*OpenRouterSeedGenerator, error) {
	if cfg.APIKey == "" {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("openrouter API key is required"))
	}
	if cfg.Model == "" {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("openrouter model is required"))
	}
	if cfg.BaseURL == "" {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("openrouter base URL is required"))
	}
	if cfg.PromptsDir == "" {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("prompts dir is required"))
	}
	if cfg.Timeout <= 0 {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("timeout must be > 0"))
	}
	if cfg.MaxRetries < 0 {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("max retries must be >= 0"))
	}
	if len(cfg.Phases) == 0 {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("at least one phase is required"))
	}
	for _, phase := range cfg.Phases {
		if _, err := phaseTemplateFile(phase); err != nil {
			return nil, err
		}
	}
	if cfg.RetryDelay < 0 {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("retry delay must be >= 0"))
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}

	return &OpenRouterSeedGenerator{
		cfg:        cfg,
		httpClient: httpClient,
		sleep:      time.Sleep,
	}, nil
}

func (g *OpenRouterSeedGenerator) Generate(ctx context.Context, req Request) (*GeneratedCode, error) {
	if len(req.ScenarioYAML) == 0 {
		return nil, NewGenerateError(ErrGenerateFailed, "request", fmt.Errorf("scenario YAML is required"))
	}

	phaseResults := make([]PhaseResult, 0, len(g.cfg.Phases))
	phaseOutput := map[string]string{}
	lastFiles := map[string][]byte{}
	var filteredSchema string

	for i, phase := range g.cfg.Phases {
		prompt, err := g.renderPhasePrompt(phase, req, phaseOutput, lastFiles, filteredSchema)
		if err != nil {
			return nil, err
		}

		text, err := g.runPhaseWithRetries(ctx, phase, prompt)
		if err != nil {
			return nil, err
		}

		phaseOutput[phase] = text
		phaseResults = append(phaseResults, PhaseResult{
			Name:   phase,
			Prompt: []byte(prompt),
			Output: []byte(text),
		})

		switch phase {
		case PhasePlanArchitecture:
			if len(req.ProviderSchemaJSON) > 0 {
				resourceTypes, extractErr := ExtractResourceTypesFromArchitecturePlan(text)
				if extractErr == nil && len(resourceTypes) > 0 {
					schema, filterErr := FilterSchemaForResourceTypes(req.ProviderSchemaJSON, resourceTypes)
					if filterErr == nil {
						filteredSchema = schema
					}
				}
			}
		case PhaseGenerateHCL:
			files, parseErr := ParseFileBlocks(text)
			if parseErr != nil {
				return nil, NewGenerateError(ErrParseFailed, phase, parseErr)
			}
			lastFiles = files
		case PhaseSelfReview:
			if SelfReviewIndicatesNoChanges(text) {
				break
			}
			files, parseErr := ParseFileBlocks(text)
			if parseErr != nil {
				// Self-review produced unparseable output — treat as no-op.
				break
			}
			if lastFiles == nil {
				lastFiles = make(map[string][]byte, len(files))
			}
			for name, content := range files {
				lastFiles[name] = content
			}
		}

		if i < len(g.cfg.Phases)-1 && g.cfg.PhaseDelay > 0 {
			g.sleep(g.cfg.PhaseDelay)
		}
	}

	result := &GeneratedCode{
		Files: lastFiles,
		Metadata: GenerationMetadata{
			Generator: AgentTypeOpenRouter,
			Phases:    phaseResults,
		},
	}
	if err := result.Validate(); err != nil {
		return nil, NewGenerateError(ErrParseFailed, "finalize", err)
	}
	return result, nil
}

func (g *OpenRouterSeedGenerator) renderPhasePrompt(phase string, req Request, outputs map[string]string, files map[string][]byte, filteredSchema string) (string, error) {
	fileName, err := phaseTemplateFile(phase)
	if err != nil {
		return "", err
	}
	templatePath := filepath.Join(g.cfg.PromptsDir, fileName)
	pitfalls, _ := LoadPitfalls(g.cfg.PitfallsDir, req.Cloud)

	ctx := PromptContext{
		ScenarioYAML:       string(req.ScenarioYAML),
		Constraints:        g.cfg.Constraints,
		ResolvedMappings:   g.cfg.ResolvedMappings,
		Overrides:          g.cfg.Overrides,
		ArchitecturePlan:   outputs[PhasePlanArchitecture],
		AcceptanceCriteria: g.cfg.Acceptance,
		GeneratedFiles:     renderGeneratedFiles(files),
		FeedbackJSON:       string(req.FeedbackJSON),
		ProviderSchema:     filteredSchema,
		Layer3Guidance:     layer3Guidance(req.Layer3Enabled),
		Pitfalls:           pitfalls,
	}
	return RenderPromptFile(phase, templatePath, ctx)
}

func (g *OpenRouterSeedGenerator) runPhaseWithRetries(ctx context.Context, phase string, prompt string) (string, error) {
	var lastErr error
	attempts := g.cfg.MaxRetries + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		text, retryable, err := g.runOpenRouterRequest(ctx, prompt)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if !retryable || attempt == attempts {
			break
		}
		if g.cfg.RetryDelay > 0 {
			g.sleep(g.cfg.RetryDelay)
		}
	}

	return "", NewGenerateError(ErrTransportFailed, phase, lastErr)
}

func (g *OpenRouterSeedGenerator) runOpenRouterRequest(ctx context.Context, prompt string) (string, bool, error) {
	payload := map[string]any{
		"model": g.cfg.Model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", false, fmt.Errorf("marshal openrouter request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(g.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", false, fmt.Errorf("build openrouter request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		detail := redactTransportDetail(err.Error(), prompt, nil, g.cfg.APIKey)
		return "", true, fmt.Errorf("openrouter request failed: %s", detail)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		detail := redactTransportDetail(err.Error(), prompt, nil, g.cfg.APIKey)
		return "", true, fmt.Errorf("read openrouter response: %s", detail)
	}

	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		detail := redactTransportDetail(string(respBody), prompt, nil, g.cfg.APIKey)
		return "", true, fmt.Errorf("openrouter status %d: %s", resp.StatusCode, detail)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := redactTransportDetail(string(respBody), prompt, nil, g.cfg.APIKey)
		return "", false, fmt.Errorf("openrouter status %d: %s", resp.StatusCode, detail)
	}

	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		detail := redactTransportDetail(err.Error(), prompt, nil, g.cfg.APIKey)
		return "", false, fmt.Errorf("decode openrouter response: %s", detail)
	}
	if len(decoded.Choices) == 0 || decoded.Choices[0].Message.Content == "" {
		return "", false, fmt.Errorf("openrouter response missing choice content")
	}

	return decoded.Choices[0].Message.Content, false, nil
}
