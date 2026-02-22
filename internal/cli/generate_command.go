package cli

import (
	"context"
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

	scenarioPayload, err := os.ReadFile(scenarioPath)
	if err != nil {
		return fmt.Errorf("read scenario %q: %w", scenarioPath, err)
	}

	if runtime.Deps.Generator == nil {
		return fmt.Errorf("generator dependency unavailable: %w", ErrDependencyUnavailable)
	}

	generated, err := runtime.Deps.Generator.Generate(context.Background(), generator.Request{
		ScenarioPath: scenarioPath,
		ScenarioYAML: scenarioPayload,
		Iteration:    1,
	})
	if err != nil {
		return fmt.Errorf("generate code: %w", err)
	}
	if err := generated.Validate(); err != nil {
		return fmt.Errorf("validate generated files: %w", err)
	}

	writtenFiles, err := writeGeneratedFiles(runtime.OutputDir(), generated.Files)
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
