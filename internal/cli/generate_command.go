package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/redscaresu/infrafactory/internal/config"
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

func ensureGoogleProviderWiring(files map[string][]byte, cfg config.Config) {
	hasGoogleResource, hasRequiredProviders, hasProviderBlock := detectGoogleProviderWiring(files)
	if !hasGoogleResource {
		return
	}

	// If fakegcp is configured, ALWAYS rewrite the provider "google" {}
	// block — terraform-provider-google reads endpoint URLs from
	// per-service *_custom_endpoint fields on the block. A partial
	// provider block from the LLM (missing the endpoints) sends every
	// API call to api.googleapis.com instead of fakegcp. Same pattern
	// as ensureAwsProviderWiring's strip-and-replace.
	if hasProviderBlock && strings.TrimSpace(cfg.Fakegcp.URL) != "" {
		stripGoogleProviderBlock(files)
		hasProviderBlock = false
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
		sections = append(sections, buildGoogleProviderBlock(cfg.Fakegcp.URL))
	}
	injected := strings.Join(sections, "\n\n")
	if existing, ok := files["providers.tf"]; ok && strings.TrimSpace(string(existing)) != "" {
		files["providers.tf"] = []byte(strings.TrimSpace(string(existing)) + "\n\n" + injected + "\n")
		return
	}
	files["providers.tf"] = []byte(injected + "\n")
}

// buildGoogleProviderBlock emits the `provider "google" {}` block.
// When fakegcpURL is non-empty (the normal in-process Layer-2
// configuration), the block embeds per-service *_custom_endpoint
// overrides pointing at fakegcp — terraform-provider-google reads
// each service's API endpoint from these fields. Without them the
// provider tries real ADC against api.googleapis.com.
//
// When fakegcpURL is empty (a degenerate config), emit the bare
// block so the file still parses; the apply will fail later at
// validate when the provider can't auth.
func buildGoogleProviderBlock(fakegcpURL string) string {
	fakegcpURL = strings.TrimRight(fakegcpURL, "/")
	if fakegcpURL == "" {
		return `provider "google" {}`
	}
	return fmt.Sprintf(`provider "google" {
  project                                = "infrafactory-test"
  compute_custom_endpoint                = "%[1]s/compute/v1/"
  container_custom_endpoint              = "%[1]s/"
  cloud_resource_manager_custom_endpoint = "%[1]s/v1/"
  iam_custom_endpoint                    = "%[1]s/v1/"
  storage_custom_endpoint                = "%[1]s/storage/v1/"
  sql_custom_endpoint                    = "%[1]s/sql/v1beta4/"
  pubsub_custom_endpoint                 = "%[1]s/v1/"
  dns_custom_endpoint                    = "%[1]s/dns/v1/"
  cloud_run_v2_custom_endpoint           = "%[1]s/v2/"
  secret_manager_custom_endpoint         = "%[1]s/v1/"
  service_usage_custom_endpoint          = "%[1]s/v1/"
}`, fakegcpURL)
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

// ensureAwsProviderWiring is the AWS counterpart to
// ensureScalewayProviderWiring / ensureGoogleProviderWiring.
//
// Unlike Scaleway (which gets its endpoint via SCW_API_URL env var),
// terraform-provider-aws reads endpoint URLs from the
// `provider "aws" { endpoints { ... } }` block. The block is also
// where we set skip_credentials_validation / skip_metadata_api_check
// / skip_requesting_account_id so the SDK doesn't try to call real
// AWS STS GetCallerIdentity (which would 403 against fakeaws). Same
// pattern internal/e2e/aws_full_stack_test.go::awsProviderTF uses
// for hand-rolled HCL.
//
// Endpoint URLs come from cfg.Fakeaws.URL (per-service path prefixes
// match fakeaws's chi router from cmd/fakeaws/main.go); S3 goes to
// cfg.S3.URL (the SeaweedFS gateway, M59).
func ensureAwsProviderWiring(files map[string][]byte, cfg config.Config) {
	hasAwsResource, hasRequiredProviders, hasProviderBlock := detectAwsProviderWiring(files)
	if !hasAwsResource {
		return
	}

	// If fakeaws is configured, ALWAYS rewrite the provider "aws" {}
	// block to point at it — the LLM frequently emits a partial provider
	// block (e.g. missing endpoints, missing skip_credentials_validation)
	// that lets the provider escape to real AWS STS. Stripping +
	// re-injecting is the only reliable way to keep apply hermetic.
	// For required_providers, we only add when missing (the LLM-emitted
	// version is usually correct).
	if hasProviderBlock && strings.TrimSpace(cfg.Fakeaws.URL) != "" {
		stripAwsProviderBlock(files)
		hasProviderBlock = false
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
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
}`)
	}
	if missingProviderBlock {
		sections = append(sections, buildAwsProviderBlock(cfg.Fakeaws.URL, cfg.S3.URL))
	}
	injected := strings.Join(sections, "\n\n")
	if existing, ok := files["providers.tf"]; ok && strings.TrimSpace(string(existing)) != "" {
		files["providers.tf"] = []byte(strings.TrimSpace(string(existing)) + "\n\n" + injected + "\n")
		return
	}
	files["providers.tf"] = []byte(injected + "\n")
}

// stripAwsProviderBlock removes any `provider "aws" { ... }` blocks
// from every file in the working set. Thin wrapper around
// stripProviderBlock so call sites read naturally.
func stripAwsProviderBlock(files map[string][]byte) {
	stripProviderBlock(files, "aws")
}

// stripGoogleProviderBlock — same as stripAwsProviderBlock but for the
// `provider "google" { ... }` blocks. Used when fakegcp is configured
// and we need to inject *_custom_endpoint overrides; the LLM-emitted
// block typically lacks them and would let calls escape to
// api.googleapis.com.
func stripGoogleProviderBlock(files map[string][]byte) {
	stripProviderBlock(files, "google")
}

// stripProviderBlock removes every `provider "<name>" { ... }` block
// from every file in the working set. Uses brace-matching (not regex)
// so nested blocks like `endpoints { ... }` don't trip up the parser.
// After stripping, the caller's re-injection path adds the canonical
// test-mode version.
func stripProviderBlock(files map[string][]byte, providerName string) {
	marker := fmt.Sprintf(`provider %q`, providerName)
	for name, content := range files {
		text := string(content)
		out := strings.Builder{}
		i := 0
		for i < len(text) {
			idx := strings.Index(text[i:], marker)
			if idx == -1 {
				out.WriteString(text[i:])
				break
			}
			start := i + idx
			braceStart := strings.Index(text[start:], "{")
			if braceStart == -1 {
				out.WriteString(text[i : start+len(marker)])
				i = start + len(marker)
				continue
			}
			braceStart += start
			depth := 0
			end := -1
			for j := braceStart; j < len(text); j++ {
				switch text[j] {
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						end = j
					}
				}
				if end != -1 {
					break
				}
			}
			if end == -1 {
				out.WriteString(text[i:])
				break
			}
			out.WriteString(text[i:start])
			i = end + 1
			if i < len(text) && text[i] == '\n' {
				i++
			}
		}
		files[name] = []byte(out.String())
	}
}

// buildAwsProviderBlock emits the `provider "aws" {}` block with the
// test-mode flags + endpoints map that point at fakeaws (and
// optionally SeaweedFS for S3, M59). When fakeawsURL is empty the
// block falls back to a bare `provider "aws" { region = "us-east-1" }`
// so the file parses; the apply will fail at validate when the
// provider can't auth.
func buildAwsProviderBlock(fakeawsURL, s3URL string) string {
	fakeawsURL = strings.TrimRight(fakeawsURL, "/")
	if fakeawsURL == "" {
		return `provider "aws" {
  region = "us-east-1"
}`
	}
	s3Endpoint := strings.TrimRight(s3URL, "/")
	if s3Endpoint == "" {
		s3Endpoint = fakeawsURL + "/s3"
	}
	return fmt.Sprintf(`provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  s3_use_path_style           = true
  endpoints {
    iam            = "%[1]s/iam"
    ec2            = "%[1]s/ec2/region/us-east-1"
    eks            = "%[1]s/eks/region/us-east-1"
    rds            = "%[1]s/rds/region/us-east-1"
    sqs            = "%[1]s/sqs/region/us-east-1"
    dynamodb       = "%[1]s/dynamodb/region/us-east-1"
    secretsmanager = "%[1]s/secretsmanager/region/us-east-1"
    route53        = "%[1]s/route53"
    s3             = "%[2]s"
  }
}`, fakeawsURL, s3Endpoint)
}

func validateAwsProviderWiring(files map[string][]byte) error {
	hasAwsResource, hasRequiredProviders, hasProviderBlock := detectAwsProviderWiring(files)

	if !hasAwsResource {
		return nil
	}
	if !hasRequiredProviders {
		return fmt.Errorf("aws resources detected but required_providers.aws is missing")
	}
	if !hasProviderBlock {
		return fmt.Errorf("aws resources detected but provider \"aws\" block is missing")
	}
	return nil
}

func detectAwsProviderWiring(files map[string][]byte) (bool, bool, bool) {
	hasAwsResource := false
	hasRequiredProviders := false
	hasProviderBlock := false

	for _, content := range files {
		text := strings.ToLower(string(content))
		if strings.Contains(text, "aws_") {
			hasAwsResource = true
		}
		if strings.Contains(text, "required_providers") && strings.Contains(text, "\"hashicorp/aws\"") {
			hasRequiredProviders = true
		}
		if strings.Contains(text, `provider "aws"`) {
			hasProviderBlock = true
		}
	}
	return hasAwsResource, hasRequiredProviders, hasProviderBlock
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
	ensureGoogleProviderWiring(generated.Files, runtime.Config)
	if err := validateGoogleProviderWiring(generated.Files); err != nil {
		return 0, nil, fmt.Errorf("validate generated files: %w", err)
	}
	ensureAwsProviderWiring(generated.Files, runtime.Config)
	if err := validateAwsProviderWiring(generated.Files); err != nil {
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
