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
	"gopkg.in/yaml.v3"
)

func runGenerateCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	scenarioPath := args[0]
	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return fmt.Errorf("load scenario %q: %w", scenarioPath, err)
	}

	writtenFiles, err := generateAndWriteFiles(cmd.Context(), runtime, scenarioPath, 1, nil, generatedFileWriteModeClean)
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

func ensureGoogleProviderWiring(files map[string][]byte) {
	hasGoogleResource, hasRequiredProviders, hasProviderBlock := detectGoogleProviderWiring(files)
	if !hasGoogleResource {
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
    google = {
      source = "hashicorp/google"
    }
  }
}`)
	}
	if missingProviderBlock {
		sections = append(sections, `provider "google" {}`)
	}
	injected := strings.Join(sections, "\n\n")
	if existing, ok := files["providers.tf"]; ok && strings.TrimSpace(string(existing)) != "" {
		files["providers.tf"] = []byte(strings.TrimSpace(string(existing)) + "\n\n" + injected + "\n")
		return
	}
	files["providers.tf"] = []byte(injected + "\n")
}

func validateGoogleProviderWiring(files map[string][]byte) error {
	hasGoogleResource, hasRequiredProviders, hasProviderBlock := detectGoogleProviderWiring(files)

	if !hasGoogleResource {
		return nil
	}
	if !hasRequiredProviders {
		return fmt.Errorf("google resources detected but required_providers.google is missing")
	}
	if !hasProviderBlock {
		return fmt.Errorf("google resources detected but provider \"google\" block is missing")
	}
	return nil
}

func detectGoogleProviderWiring(files map[string][]byte) (bool, bool, bool) {
	hasGoogleResource := false
	hasRequiredProviders := false
	hasProviderBlock := false

	for _, content := range files {
		text := strings.ToLower(string(content))
		if strings.Contains(text, "google_") {
			hasGoogleResource = true
		}
		if strings.Contains(text, "required_providers") && strings.Contains(text, "google") {
			hasRequiredProviders = true
		}
		if strings.Contains(text, `provider "google"`) {
			hasProviderBlock = true
		}
	}
	return hasGoogleResource, hasRequiredProviders, hasProviderBlock
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

func generateAndWriteFiles(ctx context.Context, runtime *CommandRuntime, scenarioPath string, iteration int, feedbackFailures []FailureSummary, writeMode generatedFileWriteMode) (int, error) {
	written, _, err := generateAndWriteFilesWithResult(ctx, runtime, scenarioPath, iteration, feedbackFailures, writeMode)
	return written, err
}

func generateAndWriteFilesWithResult(ctx context.Context, runtime *CommandRuntime, scenarioPath string, iteration int, feedbackFailures []FailureSummary, writeMode generatedFileWriteMode) (int, *generator.GeneratedCode, error) {
	scenarioPayload, err := os.ReadFile(scenarioPath)
	if err != nil {
		return 0, nil, fmt.Errorf("read scenario %q: %w", scenarioPath, err)
	}
	if runtime.Deps.Generator == nil {
		return 0, nil, fmt.Errorf("generator dependency unavailable: %w", ErrDependencyUnavailable)
	}

	// Parse the scenario's cloud BEFORE extracting the provider schema —
	// the schema dispatcher needs sc.Cloud to pick the right provider
	// binary (scaleway/scaleway vs hashicorp/google vs hashicorp/aws).
	// Per-cloud caching inside EnsureProviderSchema keeps this O(1) for
	// repeat visits within a single process.
	var scenarioMeta struct {
		Cloud string `yaml:"cloud"`
	}
	_ = yaml.Unmarshal(scenarioPayload, &scenarioMeta)

	runtime.EnsureProviderSchema(ctx, scenarioMeta.Cloud)

	var feedbackPayload []byte
	if len(feedbackFailures) > 0 {
		feedbackPayload, err = json.Marshal(struct {
			Failures []feedbackFailure `json:"failures"`
		}{
			Failures: toFeedbackFailuresPayload(feedbackFailures),
		})
		if err != nil {
			return 0, nil, fmt.Errorf("encode generate feedback payload: %w", err)
		}
	}

	generated, err := runtime.Deps.Generator.Generate(ctx, generator.Request{
		ScenarioPath:       scenarioPath,
		ScenarioYAML:       scenarioPayload,
		FeedbackJSON:       feedbackPayload,
		Iteration:          iteration,
		ProviderSchemaJSON: runtime.ProviderSchemaJSON,
		Layer3Enabled:      runtime.Config.Validation.Layers.SandboxDeploy.Enabled,
		Cloud:              scenarioMeta.Cloud,
	})
	if err != nil {
		return 0, nil, fmt.Errorf("generate code: %w", err)
	}
	if err := generated.Validate(); err != nil {
		return 0, nil, fmt.Errorf("validate generated files: %w", err)
	}
	ensureScalewayProviderWiring(generated.Files)
	if err := validateScalewayProviderWiring(generated.Files); err != nil {
		return 0, nil, fmt.Errorf("validate generated files: %w", err)
	}
	ensureGoogleProviderWiring(generated.Files)
	if err := validateGoogleProviderWiring(generated.Files); err != nil {
		return 0, nil, fmt.Errorf("validate generated files: %w", err)
	}
	written, err := writeGeneratedFiles(runtime.OutputDir(), generated.Files, writeMode)
	if err != nil {
		return 0, nil, err
	}
	if runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		if err := validateLayer3ProjectResource(runtime.OutputDir()); err != nil {
			return 0, nil, err
		}
	}
	return written, generated, nil
}

func validateLayer3ProjectResource(outputDir string) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("read output directory for layer 3 validation: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(outputDir, entry.Name()))
		if err != nil {
			continue
		}
		if strings.Contains(string(content), `resource "scaleway_account_project"`) {
			return nil
		}
	}
	return fmt.Errorf("layer 3 requires a scaleway_account_project resource in the generated HCL for self-managed project lifecycle")
}

type generatedFileWriteMode string

const (
	generatedFileWriteModeClean       generatedFileWriteMode = "clean"
	generatedFileWriteModeIncremental generatedFileWriteMode = "incremental"
)

func writeGeneratedFiles(outputDir string, files map[string][]byte, mode generatedFileWriteMode) (int, error) {
	switch mode {
	case generatedFileWriteModeIncremental:
		if err := resetGeneratedFilesIncremental(outputDir); err != nil {
			return 0, err
		}
	default:
		if err := os.RemoveAll(outputDir); err != nil {
			return 0, fmt.Errorf("reset output directory %q: %w", outputDir, err)
		}
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

func resetGeneratedFilesIncremental(outputDir string) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read output directory %q: %w", outputDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".tf") && !strings.HasSuffix(name, ".tf.json") {
			continue
		}
		if err := os.Remove(filepath.Join(outputDir, name)); err != nil {
			return fmt.Errorf("remove generated file %q: %w", filepath.Join(outputDir, name), err)
		}
	}
	return nil
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
