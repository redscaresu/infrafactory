package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/runstore"
)

const (
	llmRawCaptureEnvVar         = "INFRAFACTORY_CAPTURE_LLM_RAW"
	llmRawResponseSchemaVersion = "infrafactory.run.llm_raw.v1"
	llmPromptSchemaVersion      = "infrafactory.run.llm_prompt.v1"
	llmRawResponseMaxBytes      = 64 * 1024
	llmRawResponseTruncatedMark = "\n[TRUNCATED]\n"
)

var nonArtifactPhaseChars = regexp.MustCompile(`[^a-z0-9_-]+`)

type llmRawResponseArtifact struct {
	Schema        string `json:"schema"`
	Iteration     int    `json:"iteration"`
	Phase         string `json:"phase"`
	RawBytes      int    `json:"raw_bytes"`
	RedactedBytes int    `json:"redacted_bytes"`
	CapturedBytes int    `json:"captured_bytes"`
	MaxBytes      int    `json:"max_bytes"`
	Truncated     bool   `json:"truncated"`
	Content       string `json:"content"`
}

type llmPromptArtifact struct {
	Schema        string `json:"schema"`
	Iteration     int    `json:"iteration"`
	Phase         string `json:"phase"`
	RawBytes      int    `json:"raw_bytes"`
	RedactedBytes int    `json:"redacted_bytes"`
	CapturedBytes int    `json:"captured_bytes"`
	MaxBytes      int    `json:"max_bytes"`
	Truncated     bool   `json:"truncated"`
	Content       string `json:"content"`
}

func llmRawCaptureEnabled() bool {
	return os.Getenv(llmRawCaptureEnvVar) == "1"
}

func persistLLMRawPhaseResponses(store *runstore.FilesystemStore, scenario, runID string, iteration int, phases []generator.PhaseResult) error {
	for _, phase := range phases {
		if phase.Name == "" {
			continue
		}

		redacted := []byte(generator.RedactSecretLikeText(string(phase.Output)))
		captured, truncated := truncateLLMRawResponse(redacted, llmRawResponseMaxBytes)

		payload, err := json.MarshalIndent(llmRawResponseArtifact{
			Schema:        llmRawResponseSchemaVersion,
			Iteration:     iteration,
			Phase:         phase.Name,
			RawBytes:      len(phase.Output),
			RedactedBytes: len(redacted),
			CapturedBytes: len(captured),
			MaxBytes:      llmRawResponseMaxBytes,
			Truncated:     truncated,
			Content:       string(captured),
		}, "", "  ")
		if err != nil {
			return fmt.Errorf("encode llm raw artifact for phase %q: %w", phase.Name, err)
		}

		name := fmt.Sprintf("llm_raw_%s.json", llmRawArtifactPhaseName(phase.Name))
		if err := store.WriteIterationArtifact(scenario, runID, iteration, name, payload); err != nil {
			return fmt.Errorf("write llm raw artifact %q: %w", name, err)
		}

		if len(phase.Prompt) > 0 {
			promptRedacted := []byte(generator.RedactSecretLikeText(string(phase.Prompt)))
			promptCaptured, promptTruncated := truncateLLMRawResponse(promptRedacted, llmRawResponseMaxBytes)
			promptPayload, err := json.MarshalIndent(llmPromptArtifact{
				Schema:        llmPromptSchemaVersion,
				Iteration:     iteration,
				Phase:         phase.Name,
				RawBytes:      len(phase.Prompt),
				RedactedBytes: len(promptRedacted),
				CapturedBytes: len(promptCaptured),
				MaxBytes:      llmRawResponseMaxBytes,
				Truncated:     promptTruncated,
				Content:       string(promptCaptured),
			}, "", "  ")
			if err != nil {
				return fmt.Errorf("encode llm prompt artifact for phase %q: %w", phase.Name, err)
			}

			promptName := fmt.Sprintf("llm_prompt_%s.json", llmRawArtifactPhaseName(phase.Name))
			if err := store.WriteIterationArtifact(scenario, runID, iteration, promptName, promptPayload); err != nil {
				return fmt.Errorf("write llm prompt artifact %q: %w", promptName, err)
			}
		}
	}

	return nil
}

func truncateLLMRawResponse(in []byte, limit int) ([]byte, bool) {
	if limit <= 0 || len(in) <= limit {
		return in, false
	}

	marker := []byte(llmRawResponseTruncatedMark)
	if len(marker) >= limit {
		return marker[:limit], true
	}

	out := make([]byte, 0, limit)
	out = append(out, in[:limit-len(marker)]...)
	out = append(out, marker...)
	return out, true
}

func llmRawArtifactPhaseName(phase string) string {
	lowered := strings.ToLower(strings.TrimSpace(phase))
	cleaned := nonArtifactPhaseChars.ReplaceAllString(lowered, "_")
	cleaned = strings.Trim(cleaned, "_")
	if cleaned == "" {
		return "unknown_phase"
	}
	return cleaned
}
