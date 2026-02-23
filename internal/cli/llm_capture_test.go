package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/runstore"
)

func TestLLMRawCaptureEnabled(t *testing.T) {
	if llmRawCaptureEnabled() {
		t.Fatal("expected disabled by default")
	}

	t.Setenv(llmRawCaptureEnvVar, "1")
	if !llmRawCaptureEnabled() {
		t.Fatal("expected enabled when env var is 1")
	}
}

func TestPersistLLMRawPhaseResponsesWritesRedactedTruncatedArtifacts(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	secretTail := strings.Repeat("x", llmRawResponseMaxBytes)
	phaseOutput := `Authorization: Bearer top-secret-token
token: "shhhh"
api_key=sk-or-v1-super-secret
` + secretTail

	phases := []generator.PhaseResult{
		{Name: "generate_hcl", Prompt: []byte("token=phase-prompt-secret\nfix validation errors"), Output: []byte(phaseOutput)},
		{Name: "Self Review/Phase", Prompt: []byte("NO ISSUES FOUND"), Output: []byte("NO ISSUES FOUND")},
	}

	if err := persistLLMRawPhaseResponses(store, "web-app-paris", "run-123", 2, phases); err != nil {
		t.Fatalf("persist llm raw phase responses: %v", err)
	}

	for _, tc := range []struct {
		file  string
		phase string
	}{
		{file: "llm_raw_generate_hcl.json", phase: "generate_hcl"},
		{file: "llm_prompt_generate_hcl.json", phase: "generate_hcl"},
		{file: "llm_raw_self_review_phase.json", phase: "Self Review/Phase"},
		{file: "llm_prompt_self_review_phase.json", phase: "Self Review/Phase"},
	} {
		path := filepath.Join(store.Root, "web-app-paris", "run-123", "iterations", "2", tc.file)
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read artifact %q: %v", tc.file, err)
		}

		var artifact llmRawResponseArtifact
		if err := json.Unmarshal(payload, &artifact); err != nil {
			t.Fatalf("decode artifact %q: %v", tc.file, err)
		}

		expectedSchema := llmRawResponseSchemaVersion
		if strings.HasPrefix(tc.file, "llm_prompt_") {
			expectedSchema = llmPromptSchemaVersion
		}
		if artifact.Schema != expectedSchema {
			t.Fatalf("expected schema %q for %q, got %q", expectedSchema, tc.file, artifact.Schema)
		}
		if artifact.Iteration != 2 {
			t.Fatalf("expected iteration 2 for %q, got %d", tc.file, artifact.Iteration)
		}
		if artifact.Phase != tc.phase {
			t.Fatalf("expected phase %q for %q, got %q", tc.phase, tc.file, artifact.Phase)
		}
		if artifact.CapturedBytes > artifact.MaxBytes {
			t.Fatalf("expected captured bytes <= max bytes for %q, got %d > %d", tc.file, artifact.CapturedBytes, artifact.MaxBytes)
		}

		if tc.phase == "generate_hcl" && strings.HasPrefix(tc.file, "llm_raw_") {
			if !artifact.Truncated {
				t.Fatalf("expected truncation for %q", tc.file)
			}
			if strings.Contains(artifact.Content, "top-secret-token") || strings.Contains(artifact.Content, "shhhh") {
				t.Fatalf("expected secret redaction in %q, got %q", tc.file, artifact.Content)
			}
			if !strings.Contains(artifact.Content, "[REDACTED]") {
				t.Fatalf("expected redaction marker in %q", tc.file)
			}
			if !strings.HasSuffix(artifact.Content, llmRawResponseTruncatedMark) {
				t.Fatalf("expected truncation marker suffix in %q", tc.file)
			}
		}

		if tc.file == "llm_prompt_generate_hcl.json" {
			if strings.Contains(artifact.Content, "phase-prompt-secret") {
				t.Fatalf("expected prompt secret redaction in %q, got %q", tc.file, artifact.Content)
			}
			if !strings.Contains(artifact.Content, "[REDACTED]") {
				t.Fatalf("expected prompt redaction marker in %q", tc.file)
			}
		}
	}
}

func TestLLMRawArtifactPhaseNameSanitization(t *testing.T) {
	t.Parallel()

	if got := llmRawArtifactPhaseName("  Self Review/Phase  "); got != "self_review_phase" {
		t.Fatalf("expected sanitized phase name self_review_phase, got %q", got)
	}
	if got := llmRawArtifactPhaseName("%%%"); got != "unknown_phase" {
		t.Fatalf("expected fallback phase name unknown_phase, got %q", got)
	}
}
