package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/spf13/cobra"
)

func runGenerateCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	scenarioPath := args[0]
	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return fmt.Errorf("load scenario %q: %w", scenarioPath, err)
	}

	writtenFiles, err := generateAndWriteFiles(cmd.Context(), runtime, scenarioPath, 1, nil)
	if err != nil {
		return err
	}

	result := OutputResult{
		Command:  "generate",
		Scenario: sc.Name,
		Status:   CommandStatusSuccess,
		Stages: []StageSummary{
			{Layer: "generate", Stage: "seed", Status: StageStatusPass},
			{Layer: "generate", Stage: "write_files", Status: StageStatusPass, Detail: fmt.Sprintf("%d files", writtenFiles)},
		},
	}

	if err := writeCommandOutput(cmd, result); err != nil {
		return err
	}

	return nil
}

func ensureScalewayProviderWiring(files map[string][]byte) {
	hasScalewayResource, hasRequiredProviders, hasProviderBlock := detectScalewayProviderWiring(files)
	if !hasScalewayResource {
		return
	}
	missingRequiredProviders := !hasRequiredProviders
	missingProviderBlock := !hasProviderBlock
	if !missingRequiredProviders && !missingProviderBlock {
		return
	}

	sections := make([]string, 0, 2)
	if missingRequiredProviders {
		sections = append(sections, `terraform {
  required_providers {
    scaleway = {
      source = "scaleway/scaleway"
    }
  }
}`)
	}
	if missingProviderBlock {
		sections = append(sections, `provider "scaleway" {}`)
	}
	injected := strings.Join(sections, "\n\n")
	if existing, ok := files["providers.tf"]; ok && strings.TrimSpace(string(existing)) != "" {
		files["providers.tf"] = []byte(strings.TrimSpace(string(existing)) + "\n\n" + injected + "\n")
		return
	}
	files["providers.tf"] = []byte(injected + "\n")
}

func validateScalewayProviderWiring(files map[string][]byte) error {
	hasScalewayResource, hasRequiredProviders, hasProviderBlock := detectScalewayProviderWiring(files)

	if !hasScalewayResource {
		return nil
	}
	if !hasRequiredProviders {
		return fmt.Errorf("scaleway resources detected but required_providers.scaleway is missing")
	}
	if !hasProviderBlock {
		return fmt.Errorf("scaleway resources detected but provider \"scaleway\" block is missing")
	}
	return nil
}

func detectScalewayProviderWiring(files map[string][]byte) (bool, bool, bool) {
	hasScalewayResource := false
	hasRequiredProviders := false
	hasProviderBlock := false

	for _, content := range files {
		text := strings.ToLower(string(content))
		if strings.Contains(text, "scaleway_") {
			hasScalewayResource = true
		}
		if strings.Contains(text, "required_providers") && strings.Contains(text, "scaleway") {
			hasRequiredProviders = true
		}
		if strings.Contains(text, `provider "scaleway"`) {
			hasProviderBlock = true
		}
	}
	return hasScalewayResource, hasRequiredProviders, hasProviderBlock
}

type feedbackFailure struct {
	Layer        string `json:"layer"`
	Stage        string `json:"stage"`
	Check        string `json:"check,omitempty"`
	Policy       string `json:"policy,omitempty"`
	Command      string `json:"command,omitempty"`
	Resource     string `json:"resource,omitempty"`
	Detail       string `json:"detail"`
	FailureClass string `json:"failure_class"`
}

func feedbackFailureClassForSummary(f FailureSummary) string {
	switch {
	case f.Check == "stuck" || f.Check == "repair_budget_exhausted" || f.Check == "target_reached":
		return "orchestration_control"
	case strings.HasPrefix(f.Check, "transport_") || strings.Contains(f.Detail, "transport"):
		return "transport_runtime"
	default:
		return "iac_validation"
	}
}

func toFeedbackFailuresPayload(in []FailureSummary) []feedbackFailure {
	out := make([]feedbackFailure, 0, len(in))
	for _, f := range in {
		out = append(out, feedbackFailure{
			Layer:        f.Layer,
			Stage:        f.Stage,
			Check:        f.Check,
			Policy:       f.Policy,
			Command:      f.Command,
			Resource:     f.Resource,
			Detail:       f.Detail,
			FailureClass: feedbackFailureClassForSummary(f),
		})
	}
	return out
}

func generateAndWriteFiles(ctx context.Context, runtime *CommandRuntime, scenarioPath string, iteration int, feedbackFailures []FailureSummary) (int, error) {
	scenarioPayload, err := os.ReadFile(scenarioPath)
	if err != nil {
		return 0, fmt.Errorf("read scenario %q: %w", scenarioPath, err)
	}
	if runtime.Deps.Generator == nil {
		return 0, fmt.Errorf("generator dependency unavailable: %w", ErrDependencyUnavailable)
	}

	var feedbackPayload []byte
	if len(feedbackFailures) > 0 {
		feedbackPayload, err = json.Marshal(struct {
			Failures []feedbackFailure `json:"failures"`
		}{
			Failures: toFeedbackFailuresPayload(feedbackFailures),
		})
		if err != nil {
			return 0, fmt.Errorf("encode generate feedback payload: %w", err)
		}
	}

	generated, err := runtime.Deps.Generator.Generate(ctx, generator.Request{
		ScenarioPath: scenarioPath,
		ScenarioYAML: scenarioPayload,
		FeedbackJSON: feedbackPayload,
		Iteration:    iteration,
	})
	if err != nil {
		return 0, fmt.Errorf("generate code: %w", err)
	}
	if err := generated.Validate(); err != nil {
		return 0, fmt.Errorf("validate generated files: %w", err)
	}
	ensureScalewayProviderWiring(generated.Files)
	if err := validateScalewayProviderWiring(generated.Files); err != nil {
		return 0, fmt.Errorf("validate generated files: %w", err)
	}
	return writeGeneratedFiles(runtime.OutputDir(), generated.Files)
}

func writeGeneratedFiles(outputDir string, files map[string][]byte) (int, error) {
	if err := os.RemoveAll(outputDir); err != nil {
		return 0, fmt.Errorf("reset output directory %q: %w", outputDir, err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return 0, fmt.Errorf("create output directory %q: %w", outputDir, err)
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		cleanName := filepath.Clean(name)
		// Reject absolute and parent-traversal paths so generated files stay
		// contained under the scenario output directory.
		if cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) || filepath.IsAbs(cleanName) {
			return 0, fmt.Errorf("invalid generated file path %q", name)
		}

		targetPath := filepath.Join(outputDir, cleanName)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return 0, fmt.Errorf("create directory for generated file %q: %w", targetPath, err)
		}
		if err := os.WriteFile(targetPath, files[name], 0o644); err != nil {
			return 0, fmt.Errorf("write generated file %q: %w", targetPath, err)
		}
	}

	return len(names), nil
}

func writeCommandOutput(cmd *cobra.Command, result OutputResult) error {
	mode, err := outputModeFromCommand(cmd)
	if err != nil {
		return err
	}

	switch mode {
	case OutputModeJSON:
		payload, err := RenderMachineJSON(result)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", payload)
	case OutputModeHuman:
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s", RenderHumanSummary(result))
	default:
		return fmt.Errorf("unsupported output mode %q", mode)
	}

	return nil
}
