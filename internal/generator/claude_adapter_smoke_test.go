package generator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestClaudeSeedGeneratorRealCommandSmoke(t *testing.T) {
	if os.Getenv("INFRAFACTORY_ENABLE_CLAUDE_TRANSPORT_SMOKE") != "1" {
		t.Skip("set INFRAFACTORY_ENABLE_CLAUDE_TRANSPORT_SMOKE=1 to enable claude transport smoke test")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Fatalf("claude binary is required for smoke test: %v", err)
	}

	dir := t.TempDir()
	promptPath := filepath.Join(dir, "phase2_generate_hcl.md")
	prompt := "Return exactly this content and nothing else:\n# File: main.tf\nterraform {}\n"
	if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}

	gen, err := NewClaudeSeedGenerator(ClaudeTransportConfig{
		Command:    "claude",
		PromptsDir: dir,
		Phases:     []string{PhaseGenerateHCL},
	}, nil)
	if err != nil {
		t.Fatalf("new generator: %v", err)
	}

	out, err := gen.Generate(context.Background(), Request{
		ScenarioYAML: []byte("scenario: smoke"),
	})
	if err != nil {
		t.Fatalf("generate smoke failed: %v", err)
	}
	if _, ok := out.Files["main.tf"]; !ok {
		t.Fatalf("expected generated main.tf file, got %+v", out.SortedFileNames())
	}
}
