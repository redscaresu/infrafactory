package generator

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenRouterSeedGeneratorGenerateSuccess(t *testing.T) {
	t.Parallel()

	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("unexpected auth header: %q", got)
		}
		var reqBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		model, _ := reqBody["model"].(string)
		if model != "anthropic/claude-3.5-sonnet" {
			t.Errorf("unexpected model: %q", model)
		}

		phaseResp := ""
		switch atomic.LoadInt32(&callCount) {
		case 1:
			phaseResp = `{"region":"fr-par"}`
		case 2:
			phaseResp = "# File: main.tf\nterraform {}"
		default:
			phaseResp = "NO ISSUES FOUND"
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": phaseResp,
					},
				},
			},
		})
	}))
	defer server.Close()

	gen := newOpenRouterGeneratorForTest(t, OpenRouterTransportConfig{
		APIKey:     "test-key",
		Model:      "anthropic/claude-3.5-sonnet",
		BaseURL:    server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 1,
		PromptsDir: writeClaudePromptFixtures(t),
		Phases: []string{
			PhasePlanArchitecture,
			PhaseGenerateHCL,
			PhaseSelfReview,
		},
	})

	out, err := gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if out.Metadata.Generator != AgentTypeOpenRouter {
		t.Fatalf("expected openrouter metadata generator, got %q", out.Metadata.Generator)
	}
	if len(out.Metadata.Phases) != 3 {
		t.Fatalf("expected 3 phase results, got %d", len(out.Metadata.Phases))
	}
	if string(out.Files["main.tf"]) != "terraform {}" {
		t.Fatalf("unexpected output file content: %q", string(out.Files["main.tf"]))
	}
}

func TestOpenRouterSeedGeneratorRetriesTransientFailures(t *testing.T) {
	t.Parallel()

	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("rate limited"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "# File: main.tf\nterraform {}",
					},
				},
			},
		})
	}))
	defer server.Close()

	gen := newOpenRouterGeneratorForTest(t, OpenRouterTransportConfig{
		APIKey:     "test-key",
		Model:      "anthropic/claude-3.5-sonnet",
		BaseURL:    server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 1,
		PromptsDir: writeSinglePhasePromptFixture(t),
		Phases:     []string{PhaseGenerateHCL},
		RetryDelay: time.Second,
	})
	var delays []time.Duration
	gen.sleep = func(d time.Duration) {
		delays = append(delays, d)
	}

	out, err := gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("expected one generated file, got %+v", out.SortedFileNames())
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Fatalf("expected two attempts, got %d", attempts)
	}
	if len(delays) != 1 || delays[0] != time.Second {
		t.Fatalf("expected retry delay of 1s, got %+v", delays)
	}
}

func TestOpenRouterSeedGeneratorNonRetryableFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid request"))
	}))
	defer server.Close()

	gen := newOpenRouterGeneratorForTest(t, OpenRouterTransportConfig{
		APIKey:     "test-key",
		Model:      "anthropic/claude-3.5-sonnet",
		BaseURL:    server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 2,
		PromptsDir: writeSinglePhasePromptFixture(t),
		Phases:     []string{PhaseGenerateHCL},
	})

	_, err := gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrTransportFailed) {
		t.Fatalf("expected transport failed error, got %v", err)
	}
}

func TestOpenRouterSeedGeneratorFailureRedactsSecretsAndPrompt(t *testing.T) {
	t.Parallel()

	secret := "sk-or-v1-super-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("prompt=scenario: smoke token=" + secret))
	}))
	defer server.Close()

	gen := newOpenRouterGeneratorForTest(t, OpenRouterTransportConfig{
		APIKey:     secret,
		Model:      "anthropic/claude-3.5-sonnet",
		BaseURL:    server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		PromptsDir: writeSinglePhasePromptFixture(t),
		Phases:     []string{PhaseGenerateHCL},
	})

	_, err := gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, secret) {
		t.Fatalf("expected api key to be redacted, got %q", msg)
	}
	if strings.Contains(msg, "scenario: smoke") {
		t.Fatalf("expected prompt text to be redacted, got %q", msg)
	}
	if !strings.Contains(msg, "[REDACTED]") {
		t.Fatalf("expected redacted marker in error, got %q", msg)
	}
}

func TestOpenRouterSeedGeneratorConfigValidation(t *testing.T) {
	t.Parallel()

	_, err := NewOpenRouterSeedGenerator(OpenRouterTransportConfig{
		APIKey:     "test-key",
		Model:      "",
		BaseURL:    "http://example.com",
		Timeout:    time.Second,
		MaxRetries: 1,
		PromptsDir: "/tmp",
		Phases:     []string{PhaseGenerateHCL},
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrTransportFailed) {
		t.Fatalf("expected transport failed error, got %v", err)
	}
}

func TestOpenRouterSeedGeneratorPhaseDelay(t *testing.T) {
	t.Parallel()

	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := atomic.AddInt32(&callCount, 1)
		content := ""
		switch attempt {
		case 1:
			content = `{"region":"fr-par"}`
		case 2:
			content = "# File: main.tf\nterraform {}"
		default:
			content = "NO ISSUES FOUND"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": content}}},
		})
	}))
	defer server.Close()

	gen := newOpenRouterGeneratorForTest(t, OpenRouterTransportConfig{
		APIKey:     "test-key",
		Model:      "anthropic/claude-3.5-sonnet",
		BaseURL:    server.URL,
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		PromptsDir: writeClaudePromptFixtures(t),
		Phases: []string{
			PhasePlanArchitecture,
			PhaseGenerateHCL,
			PhaseSelfReview,
		},
		PhaseDelay: 3 * time.Second,
	})
	var delays []time.Duration
	gen.sleep = func(d time.Duration) {
		delays = append(delays, d)
	}

	_, err := gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(delays) != 2 {
		t.Fatalf("expected two phase delays, got %d", len(delays))
	}
	for _, delay := range delays {
		if delay != 3*time.Second {
			t.Fatalf("unexpected phase delay: %v", delay)
		}
	}
}

func TestOpenRouterSeedGeneratorRealHTTPOptInSmoke(t *testing.T) {
	if os.Getenv("INFRAFACTORY_ENABLE_OPENROUTER_TRANSPORT_SMOKE") != "1" {
		t.Skip("set INFRAFACTORY_ENABLE_OPENROUTER_TRANSPORT_SMOKE=1 to enable openrouter transport smoke test")
	}

	baseURL := os.Getenv("OPENROUTER_BASE_URL")
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Fatal("OPENROUTER_API_KEY is required for openrouter smoke test")
	}
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		t.Fatal("OPENROUTER_MODEL is required for openrouter smoke test")
	}

	dir := t.TempDir()
	promptPath := filepath.Join(dir, "phase2_generate_hcl.md")
	prompt := "Return exactly this content and nothing else:\n# File: main.tf\nterraform {}\n"
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	gen := newOpenRouterGeneratorForTest(t, OpenRouterTransportConfig{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Timeout:    30 * time.Second,
		MaxRetries: 0,
		PromptsDir: dir,
		Phases:     []string{PhaseGenerateHCL},
	})

	out, err := gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err != nil {
		t.Fatalf("openrouter smoke failed: %v", err)
	}
	if _, ok := out.Files["main.tf"]; !ok {
		t.Fatalf("expected generated main.tf file, got %+v", out.SortedFileNames())
	}
}

func writeSinglePhasePromptFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	content := "Generate a file.\n# File: main.tf\nterraform {}\n"
	if err := os.WriteFile(filepath.Join(dir, "phase2_generate_hcl.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write prompt fixture: %v", err)
	}
	return dir
}

func newOpenRouterGeneratorForTest(t *testing.T, cfg OpenRouterTransportConfig) *OpenRouterSeedGenerator {
	t.Helper()

	gen, err := NewOpenRouterSeedGenerator(cfg, nil)
	if err != nil {
		t.Fatalf("new openrouter generator: %v", err)
	}
	return gen
}
