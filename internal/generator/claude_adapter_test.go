package generator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeClaudeRunner struct {
	calls   []ClaudeCommandRequest
	outputs []string
	errAt   int
	errText string
}

type blockingClaudeRunner struct{}

func (blockingClaudeRunner) Run(ctx context.Context, _ ClaudeCommandRequest) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (f *fakeClaudeRunner) Run(_ context.Context, req ClaudeCommandRequest) ([]byte, error) {
	f.calls = append(f.calls, req)
	if f.errAt > 0 && len(f.calls) == f.errAt {
		if f.errText != "" {
			return nil, errors.New(f.errText)
		}
		return nil, errors.New("boom")
	}
	if len(f.outputs) < len(f.calls) {
		return nil, fmt.Errorf("missing fake output for call %d", len(f.calls))
	}
	return []byte(f.outputs[len(f.calls)-1]), nil
}

func TestNewClaudeSeedGeneratorConfigValidation(t *testing.T) {
	t.Parallel()

	_, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command:    "claude",
		PromptsDir: "/tmp",
		Phases:     []string{"bad_phase"},
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrTransportFailed) {
		t.Fatalf("expected transport failed error, got %v", err)
	}
}

func TestClaudeSeedGeneratorGenerateDeterministicFlow(t *testing.T) {
	t.Parallel()

	promptsDir := writeClaudePromptFixtures(t)
	runner := &fakeClaudeRunner{
		outputs: []string{
			`{"region":"fr-par"}`,
			"# File: main.tf\nterraform {}",
			"NO ISSUES FOUND",
		},
	}

	gen, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command:    "claude",
		PromptsDir: promptsDir,
		Phases: []string{
			PhasePlanArchitecture,
			PhaseGenerateHCL,
			PhaseSelfReview,
		},
	}, runner)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}

	out, err := gen.Generate(context.Background(), Request{
		ScenarioYAML: []byte("scenario: smoke"),
		FeedbackJSON: []byte(`{"failures":[]}`),
		Iteration:    2,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if len(runner.calls) != 3 {
		t.Fatalf("expected 3 phase calls, got %d", len(runner.calls))
	}
	for i, call := range runner.calls {
		if call.Command != "claude" {
			t.Fatalf("expected command claude, got %q", call.Command)
		}
		if len(call.Args) != 2 || call.Args[0] != "-p" {
			t.Fatalf("expected args [-p prompt], got %+v", call.Args)
		}
		if !strings.Contains(call.Args[1], "scenario: smoke") {
			t.Fatalf("expected prompt %d to include scenario YAML", i+1)
		}
	}
	if !strings.Contains(runner.calls[1].Args[1], `{"region":"fr-par"}`) {
		t.Fatalf("expected phase 2 prompt to include architecture plan from phase 1")
	}
	if !strings.Contains(runner.calls[2].Args[1], "# File: main.tf") {
		t.Fatalf("expected phase 3 prompt to include generated files from phase 2")
	}

	if out.Metadata.Generator != AgentTypeClaudeCode {
		t.Fatalf("expected claude generator metadata, got %q", out.Metadata.Generator)
	}
	if len(out.Metadata.Phases) != 3 {
		t.Fatalf("expected 3 metadata phases, got %d", len(out.Metadata.Phases))
	}
	if string(out.Files["main.tf"]) != "terraform {}" {
		t.Fatalf("unexpected generated file content: %q", string(out.Files["main.tf"]))
	}
}

func TestClaudeSeedGeneratorSelfReviewOverridesFiles(t *testing.T) {
	t.Parallel()

	promptsDir := writeClaudePromptFixtures(t)
	runner := &fakeClaudeRunner{
		outputs: []string{
			`{"region":"fr-par"}`,
			"# File: main.tf\nterraform {}",
			"# File: main.tf\nterraform {\n  required_version = \">= 1.6\"\n}",
		},
	}

	gen, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command:    "claude",
		PromptsDir: promptsDir,
		Phases: []string{
			PhasePlanArchitecture,
			PhaseGenerateHCL,
			PhaseSelfReview,
		},
	}, runner)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}

	out, err := gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(string(out.Files["main.tf"]), "required_version") {
		t.Fatalf("expected self-review override to be applied, got %q", string(out.Files["main.tf"]))
	}
}

func TestClaudeSeedGeneratorTransportFailure(t *testing.T) {
	t.Parallel()

	promptsDir := writeClaudePromptFixtures(t)
	runner := &fakeClaudeRunner{
		outputs: []string{
			`{"region":"fr-par"}`,
			"# File: main.tf\nterraform {}",
		},
		errAt: 2,
	}
	gen, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command:    "claude",
		PromptsDir: promptsDir,
		Phases: []string{
			PhasePlanArchitecture,
			PhaseGenerateHCL,
		},
	}, runner)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}

	_, err = gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrTransportFailed) {
		t.Fatalf("expected transport failed error, got %v", err)
	}
}

func TestClaudeSeedGeneratorTransportFailureRedactsPromptAndEnvSecrets(t *testing.T) {
	t.Parallel()

	promptsDir := writeClaudePromptFixtures(t)
	secret := "token-secret-value"
	runner := &fakeClaudeRunner{
		outputs: []string{""},
		errAt:   1,
		errText: "failed with token-secret-value while handling scenario: smoke",
	}
	gen, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command:    "claude",
		PromptsDir: promptsDir,
		Phases:     []string{PhasePlanArchitecture},
		Environment: map[string]string{
			"OPENROUTER_API_KEY": secret,
		},
	}, runner)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}

	_, err = gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err == nil {
		t.Fatal("expected error")
	}
	text := err.Error()
	if strings.Contains(text, secret) {
		t.Fatalf("expected secret to be redacted, got %q", text)
	}
	if strings.Contains(text, "scenario: smoke") {
		t.Fatalf("expected prompt body to be redacted, got %q", text)
	}
}

func TestClaudeSeedGeneratorPhaseDelayBetweenCalls(t *testing.T) {
	t.Parallel()

	promptsDir := writeClaudePromptFixtures(t)
	runner := &fakeClaudeRunner{
		outputs: []string{
			`{"region":"fr-par"}`,
			"# File: main.tf\nterraform {}",
			"NO ISSUES FOUND",
		},
	}
	gen, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command:    "claude",
		PromptsDir: promptsDir,
		Phases: []string{
			PhasePlanArchitecture,
			PhaseGenerateHCL,
			PhaseSelfReview,
		},
		PhaseDelay: 2 * time.Second,
	}, runner)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}

	var delays []time.Duration
	gen.sleep = func(d time.Duration) {
		delays = append(delays, d)
	}

	_, err = gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if len(delays) != 2 {
		t.Fatalf("expected 2 delays for 3 phases, got %d", len(delays))
	}
	for _, delay := range delays {
		if delay != 2*time.Second {
			t.Fatalf("expected delay 2s, got %v", delay)
		}
	}
}

func TestClaudeSeedGeneratorPhaseTimeout(t *testing.T) {
	t.Parallel()

	promptsDir := writeClaudePromptFixtures(t)
	gen, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command:      "claude",
		PromptsDir:   promptsDir,
		Phases:       []string{PhasePlanArchitecture},
		PhaseTimeout: 20 * time.Millisecond,
	}, blockingClaudeRunner{})
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}

	_, err = gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrTransportFailed) {
		t.Fatalf("expected transport failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "phase timed out") {
		t.Fatalf("expected timeout detail in error, got %v", err)
	}
}

func TestClaudeSeedGeneratorProgressLogging(t *testing.T) {
	t.Parallel()

	promptsDir := writeClaudePromptFixtures(t)
	runner := &fakeClaudeRunner{
		outputs: []string{
			`{"region":"fr-par"}`,
			"# File: main.tf\nterraform {}",
		},
	}
	var progress bytes.Buffer
	gen, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command:        "claude",
		PromptsDir:     promptsDir,
		Phases:         []string{PhasePlanArchitecture, PhaseGenerateHCL},
		ProgressWriter: &progress,
	}, runner)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}

	_, err = gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	text := progress.String()
	for _, expected := range []string{
		`generator/claude: phase "plan_architecture" start`,
		`generator/claude: phase "plan_architecture" complete`,
		`generator/claude: phase "generate_hcl" start`,
		`generator/claude: phase "generate_hcl" complete`,
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected progress log to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestClaudeSeedGeneratorSelfReviewNoFileBlocksFallsBackToPhase2Files(t *testing.T) {
	t.Parallel()

	promptsDir := writeClaudePromptFixtures(t)
	runner := &fakeClaudeRunner{
		outputs: []string{
			`{"region":"fr-par"}`,
			"# File: main.tf\nterraform {}",
			"Review notes only; no file blocks",
		},
	}
	var progress bytes.Buffer
	gen, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command:        "claude",
		PromptsDir:     promptsDir,
		Phases:         []string{PhasePlanArchitecture, PhaseGenerateHCL, PhaseSelfReview},
		ProgressWriter: &progress,
	}, runner)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}

	out, err := gen.Generate(context.Background(), Request{ScenarioYAML: []byte("scenario: smoke")})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if string(out.Files["main.tf"]) != "terraform {}" {
		t.Fatalf("expected phase2 files to be retained, got %q", string(out.Files["main.tf"]))
	}
	if !strings.Contains(progress.String(), `fallback: no file blocks; retaining prior files`) {
		t.Fatalf("expected fallback progress log, got:\n%s", progress.String())
	}
}

func writeClaudePromptFixtures(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	mustWriteFile := func(name string, content string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	mustWriteFile("phase1_plan_architecture.md", "S1\n{{.ScenarioYAML}}\n{{.FeedbackJSON}}\n")
	mustWriteFile("phase2_generate_hcl.md", "S2\n{{.ArchitecturePlan}}\n{{.ScenarioYAML}}\n")
	mustWriteFile("phase3_self_review.md", "S3\n{{.GeneratedFiles}}\n{{.ScenarioYAML}}\n")
	return dir
}
