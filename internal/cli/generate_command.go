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

	// Even when the LLM emits a complete `terraform { required_providers
	// { google = {...} } }` block (so missingRequiredProviders is false
	// and the canonical version-pinned block below doesn't get
	// injected), the LLM frequently omits the version pin. provider-
	// google v6 drops + renames several *_custom_endpoint variables
	// fakegcp depends on; v6 callers 401 against real google APIs.
	// Surgically inject `version = "~> 5.0"` into the existing google
	// entry so this regression can't slip through.
	if hasRequiredProviders {
		ensureGoogleVersionPin(files)
	}

	missingRequiredProviders := !hasRequiredProviders
	missingProviderBlock := !hasProviderBlock
	if !missingRequiredProviders && !missingProviderBlock {
		return
	}

	sections := make([]string, 0, 2)
	if missingRequiredProviders {
		// Pin provider-google to ~> 5.0. v6 split iam_custom_endpoint
		// away from the iam.admin.v1 API path that google_service_account
		// uses, so SA create/read/delete hit real iam.googleapis.com
		// with the fake-token and 401. v6 also renames several other
		// endpoint vars in ways the LLM keeps tripping on. v5.x is the
		// last version where the single iam_custom_endpoint covers
		// every IAM resource fakegcp models.
		sections = append(sections, `terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
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
  # fakegcp's requireBearerToken middleware 401s every request that
  # arrives without an Authorization header. terraform-provider-google
  # only sets the header when access_token is configured, so this is
  # not optional — without it every API call gets the 401 OAuth
  # error that looks like the provider escaped to real google.
  access_token                           = "fake-token"
  compute_custom_endpoint                = "%[1]s/compute/v1/"
  container_custom_endpoint              = "%[1]s/"
  cloud_resource_manager_custom_endpoint = "%[1]s/v1/"
  # resource_manager_v3_custom_endpoint covers Resource Manager v3,
  # which newer v5 code paths (notably google_service_networking_connection
  # and the getProject() preflight several resources call before Read)
  # use instead of v1. Pre-Ticket-D-2 the v1 override was set but v3
  # wasn't, so the preflight escaped to real
  # cloudresourcemanager.googleapis.com and surfaced as a misleading
  # 401 ACCESS_TOKEN_TYPE_UNSUPPORTED error that LOOKED like an auth
  # issue but was actually a missing-endpoint-override.
  # Host-only (no trailing /v3/) — same shape pattern as T2 / T11.
  resource_manager_v3_custom_endpoint    = "%[1]s/"
  # iam_custom_endpoint must NOT include a trailing /v1/ — the
  # provider prepends "v1/projects/..." for google_service_account.
  # Including /v1/ here produces /v1/v1/projects/... which fakegcp
  # 501s. Confirmed by the fakegcp working/iam example.
  iam_custom_endpoint                    = "%[1]s/"
  storage_custom_endpoint                = "%[1]s/storage/v1/"
  # sql_custom_endpoint must NOT include trailing /sql/v1beta4/. The v5
  # provider's NewSqlAdminClient strips the version twice with a regex
  # that requires literal "https://" — so for our http:// fakegcp
  # endpoint the strip is a no-op and BasePath stays at
  # http://.../sql/v1beta4/. The sqladmin/v1beta4 client then
  # ResolveRelative-prepends sql/v1beta4/projects/... to that, doubling
  # to /sql/v1beta4/sql/v1beta4/projects/... which fakegcp 501s.
  # Dropping the trailing path leaves BasePath at http://.../ and the
  # prepended sql/v1beta4/projects/... lands on the registered fakegcp
  # route at /sql/v1beta4/projects/{project}.
  sql_custom_endpoint                    = "%[1]s/"
  pubsub_custom_endpoint                 = "%[1]s/v1/"
  # dns_custom_endpoint must be HOST-ONLY. terraform-provider-google
  # uses TWO call patterns for DNS: direct {{DNSBasePath}}projects/...
  # for zone CRUD (URL = ${DNSBasePath}projects/...) and the lib
  # client for record-set Changes + zone delete preflight. The lib's
  # NewDnsClient does RemoveBasePathVersion (no-op on http://) then
  # ReplaceAll("/dns/", ""), so any endpoint that contains /dns/ ends
  # up with the port mangled (e.g. ".../dns/v1/" → "...:8081v1/") and
  # googleapi.ResolveRelative panics on url.Parse, surfacing as
  # "Plugin did not respond" on google_dns_record_set. With host-only
  # endpoint, ReplaceAll is a no-op and ResolveRelative composes the
  # lib's "dns/v1/projects/..." relative path correctly. fakegcp also
  # exposes the direct-path routes at /projects/{p}/managedZones so
  # zone CRUD lands on the same handlers as /dns/v1/projects/...
  dns_custom_endpoint                    = "%[1]s/"
  cloud_run_v2_custom_endpoint           = "%[1]s/v2/"
  secret_manager_custom_endpoint         = "%[1]s/v1/"
  # service_usage_custom_endpoint must NOT include /v1/ — the provider
  # prepends "v1/projects/.../services" itself. Including /v1/ here
  # produces /v1/v1/projects/.../services which fakegcp 501s.
  service_usage_custom_endpoint          = "%[1]s/"
  # service_networking_custom_endpoint covers Private Service Access
  # (google_service_networking_connection). servicenetworking lib uses
  # v1/{+parent}/connections-style relative paths, so BasePath should
  # be host-only. Without this override, the provider 401s against
  # the real servicenetworking.googleapis.com and Cloud SQL private-IP
  # scenarios stall.
  service_networking_custom_endpoint     = "%[1]s/"
  redis_custom_endpoint                  = "%[1]s/v1/"
  # Cloud KMS — fakegcp ships stub key-ring/crypto-key handlers so
  # the gcp.encryption policy (CMEK on storage/sql/disk) can be
  # satisfied by declaring KMS resources in HCL without hitting the
  # real cloudkms.googleapis.com endpoint.
  #
  # Host-only (no trailing /v1/) — terraform-provider-google's KMS
  # client uses the v5 cloudkms lib which prepends "v1/projects/..."
  # to the BasePath itself. With "%[1]s/v1/" we'd get
  # /v1/v1/projects/... which fakegcp 501s. Same shape as T2's
  # sql_custom_endpoint and T11's dns_custom_endpoint fixes.
  # Surfaced in gcp-cloud-sql iter 5 (2026-05-31).
  kms_custom_endpoint                    = "%[1]s/"
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

// ensureGoogleVersionPin scans every file in the working set for a
// `google = { ... }` entry inside `required_providers` and injects
// `version = "~> 5.0"` when missing. provider-google v6 split / renamed
// several *_custom_endpoint variables fakegcp depends on; without a
// version pin terraform pulls v6 and the apply 401s against real
// google APIs. Surgical rewrite preserves any sibling providers
// (`random`, `time`, etc.) the LLM included.
func ensureGoogleVersionPin(files map[string][]byte) {
	for name, content := range files {
		text := string(content)
		marker := "google = {"
		out := strings.Builder{}
		i := 0
		modified := false
		for i < len(text) {
			idx := strings.Index(text[i:], marker)
			if idx == -1 {
				out.WriteString(text[i:])
				break
			}
			start := i + idx
			braceStart := start + len(marker) - 1
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
			block := text[start : end+1]
			if !strings.Contains(block, "version") {
				// Inject the version pin just before the closing brace.
				inner := text[braceStart+1 : end]
				inner = strings.TrimRight(inner, " \n\t")
				newBlock := text[start:braceStart+1] + inner + "\n      version = \"~> 5.0\"\n    }"
				out.WriteString(text[i:start])
				out.WriteString(newBlock)
				modified = true
			} else {
				out.WriteString(text[i : end+1])
			}
			i = end + 1
		}
		if modified {
			files[name] = []byte(out.String())
		}
	}
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
    kms            = "%[1]s/kms/region/us-east-1"
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
