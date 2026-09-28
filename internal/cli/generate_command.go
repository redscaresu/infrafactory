package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func runGenerateCommand(cmd *cobra.Command, args []string, runtime *CommandRuntime) error {
	scenarioPath := args[0]
	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return fmt.Errorf("load scenario %q: %w", scenarioPath, err)
	}

	writtenFiles, _, err := generateAndWriteFilesWithResult(cmd.Context(), runtime, scenarioPath, "", 1, nil, generatedFileWriteModeClean)
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
		// Pinned, because Layer 3 refuses an unpinned provider and this
		// block is what a generated stack gets when the model omitted
		// one -- injecting a version-less block would make the repair
		// loop fight a check it cannot satisfy by editing HCL.
		sections = append(sections, fmt.Sprintf(`terraform {
  required_providers {
    scaleway = {
      source  = %q
      version = %q
    }
  }
}`, layer3ScalewayProviderSource, layer3ScalewayProviderVersion))
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
  # user_project_override = false disables the x-goog-user-project
  # quota-attribution header. With the default (true), the v5 SDK
  # requires a user-account OAuth token and routes the
  # google_project_service / google_service_networking_connection
  # preflight (Projects.GetProject) through Google's token-exchange
  # path even when cloud_resource_manager_custom_endpoint is set,
  # surfacing as a 401 ACCESS_TOKEN_TYPE_UNSUPPORTED that LOOKS like
  # the request escaped to real cloud. False = service-account semantics,
  # which fakegcp's bearer-token middleware accepts directly.
  user_project_override                  = false
  # NOTE: do NOT add a credentials attribute here. The v5 provider rejects
  # any HCL that sets BOTH access_token and credentials with a fatal
  # Invalid Attribute Combination / Conflicting configuration arguments
  # error. We rely on access_token (for the bearer) + the GCP env-var
  # strip in internal/cli/exec_runner.go::stripGCPAuthEnv to keep the
  # SDK's auth pipeline from probing Application Default Credentials.
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

  # Disable global request batching. With batching enabled (default),
  # google_project_iam_member's writes are aggregated by the v5
  # provider's BatchingConfig wrapper, which constructs its OWN
  # cloudresourcemanager client. That client does NOT honor
  # cloud_resource_manager_custom_endpoint reliably — empirical
  # evidence: gcp-full-stack 2026-06-02 sweep saw
  # google_project_iam_member.* escape to real
  # cloudresourcemanager.googleapis.com with
  # ACCESS_TOKEN_TYPE_UNSUPPORTED even though every other IAM /
  # ResourceManager call landed cleanly on fakegcp. Disabling batching
  # forces each IAM mutation through the normal client path which
  # respects the endpoint override.
  batching {
    enable_batching = false
  }
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

// ensureAwsProviderWiring writes the AWS provider config itself, the same
// bytes at every layer (ADR-0039): every provider "aws" block the model
// wrote is replaced by buildAwsProviderBlock's, and required_providers aws
// is pinned to exactly harness.AWSProviderVersion. Endpoints reach the
// provider through the environment (cloudEnv), never the HCL, so neither
// fakeaws.url, s3.url nor sandbox_deploy changes a byte; aws.region
// changes the region literal. A non-empty runID adds default_tags
// carrying it, so every resource the run applies is taggable back to it;
// model HCL that names that tag itself is refused.
func ensureAwsProviderWiring(files map[string][]byte, cfg config.Config, runID string) error {
	if hasAwsResource, _, _ := detectAwsProviderWiring(files); !hasAwsResource {
		return nil
	}
	stripAwsProviderBlock(files)
	if err := refuseAwsRunIDTag(files); err != nil {
		return err
	}
	injected := buildAwsProviderBlock(awsRegion(cfg.AWS), runID)
	if !pinAwsRequiredProvider(files) {
		injected = `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "` + harness.AWSProviderVersion + `"
    }
  }
}

` + injected
	}
	if existing := strings.TrimSpace(string(files["providers.tf"])); existing != "" {
		injected = existing + "\n\n" + injected
	}
	files["providers.tf"] = []byte(injected + "\n")
	return nil
}

// refuseAwsRunIDTag refuses any generated file that names awsRunIDTagKey,
// once the model's provider blocks are gone: a resource's own tag of that
// key overrides default_tags, so the model would choose which run the
// resource belongs to. Any mention counts, not just a tags attribute,
// because the key can reach one through a local, a variable default,
// merge(), a .tfvars value or a .tf.json file. Comments in HCL files
// are not mentions.
func refuseAwsRunIDTag(files map[string][]byte) error {
	for _, name := range slices.Sorted(maps.Keys(files)) {
		if namesOutsideComments(name, files[name], awsRunIDTagKey) {
			return fmt.Errorf("%s names the %q tag, which infrafactory sets from the run's id: remove it", name, awsRunIDTagKey)
		}
	}
	return nil
}

// namesOutsideComments reports whether word appears in content other than
// in a comment. Only .tf and .tfvars files are read as HCL; any other
// file, or HCL that does not lex, counts every mention.
func namesOutsideComments(name string, content []byte, word string) bool {
	if !strings.HasSuffix(name, ".tf") && !strings.HasSuffix(name, ".tfvars") {
		return strings.Contains(string(content), word)
	}
	tokens, diags := hclsyntax.LexConfig(content, name, hcl.InitialPos)
	if diags.HasErrors() {
		return strings.Contains(string(content), word)
	}
	for _, token := range tokens {
		if token.Type != hclsyntax.TokenComment && strings.Contains(string(token.Bytes), word) {
			return true
		}
	}
	return false
}

// pinAwsRequiredProvider sets every required_providers aws entry to
// exactly source hashicorp/aws and version harness.AWSProviderVersion,
// whatever the model wrote, leaving sibling providers alone. It reports
// whether any file declared one.
func pinAwsRequiredProvider(files map[string][]byte) bool {
	pin := cty.ObjectVal(map[string]cty.Value{
		"source":  cty.StringVal("hashicorp/aws"),
		"version": cty.StringVal(harness.AWSProviderVersion),
	})
	pinned := false
	for name, content := range files {
		if !strings.HasSuffix(name, ".tf") {
			continue
		}
		file, diags := hclwrite.ParseConfig(content, name, hcl.InitialPos)
		if diags.HasErrors() {
			continue
		}
		changed := false
		for _, terraform := range file.Body().Blocks() {
			if terraform.Type() != "terraform" {
				continue
			}
			for _, required := range terraform.Body().Blocks() {
				if required.Type() == "required_providers" && required.Body().GetAttribute("aws") != nil {
					required.Body().SetAttributeValue("aws", pin)
					changed = true
				}
			}
		}
		if changed {
			files[name] = hclwrite.Format(file.Bytes())
			pinned = true
		}
	}
	return pinned
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

// awsRunIDTagKey is the tag default_tags puts on every AWS resource a run
// applies, valued with the run id; the scope sweep reads it back.
const awsRunIDTagKey = harness.AWSRunIDTagKey

// buildAwsProviderBlock is the whole AWS provider block, at both layers
// (ADR-0039 decisions 3 and 9). s3_use_path_style is the one setting with
// no environment form; everything else, endpoints and credentials
// included, comes from the environment. The run id comes from the run,
// never the model; empty (a bare `generate`) writes no default_tags.
func buildAwsProviderBlock(region, runID string) string {
	block := fmt.Sprintf(`provider "aws" {
  region            = %q
  s3_use_path_style = true
`, region)
	if runID != "" {
		block += fmt.Sprintf(`
  default_tags {
    tags = {
      %q = %q
    }
  }
`, awsRunIDTagKey, runID)
	}
	return block + "}"
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
		// Require an actual `resource "aws_…"` or `data "aws_…"`
		// declaration, not just any substring containing "aws_".
		// The looser check tripped on the genesyscloud provider's
		// `aws_region = "us-east-1"` attribute and injected an AWS
		// provider block into Genesys scenarios — duplicating
		// `terraform {}` and breaking `tofu init`. S114 dispatch
		// hardening.
		if strings.Contains(text, `resource "aws_`) || strings.Contains(text, `data "aws_`) {
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

// awsAMIForGeneration is the AMI id an AWS model writes verbatim:
// fakeaws's AL2023 answer at Layer 2, the id the run resolved from real
// SSM at Layer 3. Without a resolved id a Layer 3 run stops here, before
// any model call. Every other cloud gets "".
func awsAMIForGeneration(runtime *CommandRuntime, cloud string) (string, error) {
	if cloud != "aws" {
		return "", nil
	}
	if !runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return harness.AWSLayer2AMI, nil
	}
	if runtime.AWSLayer3AMI == "" {
		return "", errors.New("aws at Layer 3 has no resolved AMI id: the run reads it from SSM before generating, so nothing was generated")
	}
	return runtime.AWSLayer3AMI, nil
}

// runID is the run's id, or "" outside a run; it reaches the generated
// HCL only through the AWS provider block's default_tags.
func generateAndWriteFilesWithResult(ctx context.Context, runtime *CommandRuntime, scenarioPath, runID string, iteration int, feedbackFailures []FailureSummary, writeMode generatedFileWriteMode) (int, *generator.GeneratedCode, error) {
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
		// ServiceSpec has no yaml tags: yaml.v3's lowercased field names
		// match image, tag and port, which is all the script reads.
		Service *scenario.ServiceSpec `yaml:"service"`
	}
	scenarioMetaErr := yaml.Unmarshal(scenarioPayload, &scenarioMeta)

	// Rendered before generation so a refused image or tag costs no model
	// call. Only AWS boots the service from a script infrafactory writes.
	var userData []byte
	userDataLine := ""
	if scenarioMeta.Cloud == "aws" && scenarioMeta.Service != nil {
		userData, err = renderAWSUserData(*scenarioMeta.Service)
		if err != nil {
			return 0, nil, err
		}
		userDataLine = generator.AWSUserDataLine
	}
	amiID, err := awsAMIForGeneration(runtime, scenarioMeta.Cloud)
	if err != nil {
		return 0, nil, err
	}

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
		UserDataLine:       userDataLine,
		AMIID:              amiID,
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
	if err := ensureAwsProviderWiring(generated.Files, runtime.Config, runID); err != nil {
		return 0, nil, fmt.Errorf("validate generated files: %w", err)
	}
	if err := validateAwsProviderWiring(generated.Files); err != nil {
		return 0, nil, fmt.Errorf("validate generated files: %w", err)
	}
	// S122: pre-place a default flow YAML when the LLM emits a
	// genesyscloud_flow resource. The provider reads filepath at PLAN
	// time via CustomizeDiff, so no tofu pattern (local_file +
	// depends_on, null_resource provisioners) can satisfy the
	// constraint. The harness drops the file alongside the .tf so a
	// bare `filepath = "${path.module}/flow.yaml"` resolves at plan.
	ensureGenesysFlowAsset(generated.Files)
	if err := placeAWSUserData(generated.Files, scenarioMeta.Cloud, userData); err != nil {
		return 0, nil, fmt.Errorf("validate generated files: %w", err)
	}
	written, err := writeGeneratedFiles(runtime.OutputDir(), generated.Files, writeMode)
	if err != nil {
		return 0, nil, err
	}
	if runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		// Keyed on the scenario's cloud, so a payload whose cloud cannot
		// be read is refused rather than gated as Scaleway by default.
		if scenarioMetaErr != nil {
			return 0, nil, fmt.Errorf("read cloud of scenario %q for the layer 3 gate: %w", scenarioPath, scenarioMetaErr)
		}
		cloud, err := parseLayer3Cloud(scenarioMeta.Cloud)
		if err != nil {
			return 0, nil, err
		}
		if err := validateLayer3ProjectResource(runtime.OutputDir()); err != nil {
			return 0, nil, err
		}
		if err := layer3PreflightHCLForCloud(cloud, runtime.OutputDir(), runtime.Config.Validation.Layers.SandboxDeploy.AllowResourceTypes); err != nil {
			return 0, nil, err
		}
		if err := validateLayer3ResourceAllowlist(runtime.OutputDir(), runtime.Config.Validation.Layers.SandboxDeploy.AllowResourceTypes); err != nil {
			return 0, nil, err
		}
	}
	return written, generated, nil
}

// layer3ResourceRe matches a `resource "<type>" "<name>"` declaration.
// Same approach as genesysFlowResourceRe below -- the generated HCL is
// machine-written and regular, and a full HCL parse would buy nothing
// here.
var layer3ResourceRe = regexp.MustCompile(`resource\s+"([a-z0-9_]+)"\s+"[^"]+"`)

// validateLayer3ResourceAllowlist blocks expensive resource types before
// any API call is made.
//
// The iteration loop is an LLM writing HCL. Against mockway a stray
// scaleway_k8s_cluster costs nothing and gets caught; against real
// Scaleway it is a slow, expensive apply followed by a slow destroy,
// several times over as the loop iterates. Failing here -- after
// generation, before apply -- turns that into a fast structural error
// the next iteration can route around.
//
// Deny-by-default: an empty allowlist denies everything rather than
// permitting everything. A config that forgot to set the list should
// stop the run, not provision whatever was generated.
func validateLayer3ResourceAllowlist(outputDir string, allowed []string) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("read output directory for layer 3 allowlist validation: %w", err)
	}

	denied := make([]string, 0)
	seen := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(outputDir, entry.Name()))
		if err != nil {
			continue
		}
		for _, match := range layer3ResourceRe.FindAllStringSubmatch(string(content), -1) {
			resourceType := match[1]
			if _, ok := seen[resourceType]; ok {
				continue
			}
			seen[resourceType] = struct{}{}
			if !resourceTypeAllowed(resourceType, allowed) {
				denied = append(denied, resourceType)
			}
		}
	}
	if len(denied) == 0 {
		return nil
	}
	sort.Strings(denied)
	return fmt.Errorf(
		"layer 3 refuses to apply resource type(s) %s: not in validation.layers.sandbox_deploy.allow_resource_types (%s). These types are denied by default because they are slow and costly to provision against real Scaleway; either use a cheaper equivalent or widen the allowlist deliberately",
		strings.Join(denied, ", "),
		strings.Join(allowed, ", "),
	)
}

func resourceTypeAllowed(resourceType string, allowed []string) bool {
	for _, pattern := range allowed {
		if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
			if strings.HasPrefix(resourceType, prefix) {
				return true
			}
			continue
		}
		if resourceType == pattern {
			return true
		}
	}
	return false
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
		// Inverted by ADR-0025: the run's project is created before the
		// apply and handed to the provider, so generated HCL must NOT
		// declare one. A declared project would be a second project that
		// nothing tracks, no marker names and no teardown deletes.
		//
		// A substring scan is enough here because this is the generation
		// path, where the HCL came from our own prompt. The untrusted
		// path parses instead -- `resource /*x*/ "..."` is valid HCL a
		// regex does not match, which is what layer3PreflightHCL is for.
		if strings.Contains(string(content), `resource "scaleway_account_project"`) {
			return fmt.Errorf(
				"generated HCL declares a scaleway_account_project in %s; under ADR-0025 the run's project is created before the apply, so a declared one would be a second project nothing destroys",
				entry.Name())
		}
	}
	return nil
}

// genesysFlowResourceRe matches a `resource "genesyscloud_flow" "<name>"`
// declaration anywhere in the generated HCL.
var genesysFlowResourceRe = regexp.MustCompile(`resource\s+"genesyscloud_flow"\s+"[^"]+"`)

// defaultGenesysFlowYAML is a minimal inboundCall flow that the
// fakegenesys mock accepts without complaint. Real Genesys validates
// the shape strictly; the mock just stores whatever bytes arrive.
const defaultGenesysFlowYAML = `inboundCall:
  name: harness-stub-flow
  defaultLanguage: en-us
  startUpRef: "/inboundCall/menus/menu[main]"
  supportedLanguages:
    en-us:
      defaultLanguageSkill:
        noValue: true
  menus:
    - menu:
        name: main
        refId: main
        audio:
          defaultAudio:
            tts: "Hello."
        choices: []
`

// ensureGenesysFlowAsset pre-places a default `flow.yaml` in the
// generated-files map whenever the LLM emits a `genesyscloud_flow`
// resource. The provider reads `filepath` at PLAN time via
// CustomizeDiff (resource_genesyscloud_flow.go) — local_file +
// depends_on, null_resource provisioners, etc. all evaluate after
// plan, so the file MUST exist on disk before `tofu plan` runs.
//
// If the LLM already provided a `flow.yaml` (e.g. via the inline-
// generate pattern from phase2_generate_hcl.md), keep it. Otherwise
// drop the harness stub.
//
// Coordinated with prompts/genesys/phase2_generate_hcl.md item #11:
// the LLM is told to reference `"${path.module}/flow.yaml"` so this
// file resolves correctly.
func ensureGenesysFlowAsset(files map[string][]byte) {
	hasFlow := false
	for _, b := range files {
		if genesysFlowResourceRe.Match(b) {
			hasFlow = true
			break
		}
	}
	if !hasFlow {
		return
	}
	if _, ok := files["flow.yaml"]; ok {
		return
	}
	files["flow.yaml"] = []byte(defaultGenesysFlowYAML)
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
		write := os.WriteFile
		if cleanName == generator.AWSUserDataFile {
			write = writeFileExclusive
		}
		if err := write(targetPath, files[name], 0o644); err != nil {
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
		name := entry.Name()
		switch {
		case name == generator.AWSUserDataFile:
			// Replaced fresh every iteration; anything but a regular file
			// is refused rather than followed or recursed into.
			if !entry.Type().IsRegular() {
				return fmt.Errorf("refusing %q: %s is not a regular file", filepath.Join(outputDir, name), name)
			}
		case entry.IsDir():
			continue
		case !strings.HasSuffix(name, ".tf") && !strings.HasSuffix(name, ".tf.json"):
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
