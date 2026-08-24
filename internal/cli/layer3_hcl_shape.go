package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Layer 3 applies HCL that can arrive from a pull request (the S144 gate
// stages committed fixtures), in a process holding live cloud credentials.
// That is untrusted input, and it is validated by PARSING rather than by
// pattern matching.
//
// Regexes were tried and are not sound here. `resource /*x*/ "scaleway_k8s_cluster"`
// is valid HCL that a `resource\s+"` pattern misses, and no amount of
// patching the expression fixes the class of problem: the grammar permits
// comments and whitespace almost anywhere, so a scanner that is not a
// parser will always have a gap.
//
// The policy is deny-by-default on BLOCK TYPE, not a denylist of known
// escape hatches. Denylists fail the same way regexes do — `module` and
// `provisioner` were blocked, and `data "external"` with `program = [...]`
// still executed commands during plan. Enumerating what is permitted ends
// that game.
var layer3AllowedTopLevelBlocks = map[string]bool{
	"terraform": true,
	"provider":  true,
	"variable":  true,
	"output":    true,
	"locals":    true,
	"resource":  true,
	"data":      true,
}

// layer3DeniedNestedBlocks execute commands during apply, inside a process
// that holds the cloud credentials.
var layer3DeniedNestedBlocks = map[string]bool{
	"provisioner": true,
	"connection":  true,
	// lifecycle carries precondition/postcondition, whose error_message is
	// an expression evaluated at PLAN time and surfaced in tofu's output.
	// A Layer 3 stack has no need of it.
	"lifecycle": true,
	// backend is processed by `tofu init`, before any apply. A PR-chosen
	// backend would have the secret-bearing job contact it, and would move
	// state off the local terraform-live.tfstate that every cleanup and
	// sweep decision reads.
	"backend": true,
	"cloud":   true,
}

// layer3SafeProviderAttrs are the only settings a PR may put in the
// scaleway provider block.
//
// The endpoint comes from the sealed environment and nowhere else -- that
// is the whole of S139. A provider block accepting `api_url` would let a
// fixture retarget a "real" apply, and `project_id` would move resources
// out of the disposable per-run project that bounds blast radius. Region
// and zone are placement, not identity or destination.
var layer3SafeProviderAttrs = map[string]bool{
	"region": true,
	"zone":   true,
}

// validateLayer3HCLShape refuses anything a Layer 3 stack has no business
// containing, before any tofu invocation.
//
// Before, not during: `data "external"` runs its program at PLAN time, so a
// check that waits for plan output has already lost.
func validateLayer3HCLShape(outputDir string, allowedResourceTypes []string) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("read output directory for layer 3 HCL validation: %w", err)
	}
	problems := make([]string, 0)
	sawCanonicalProvider := false
	for _, entry := range entries {
		// OpenTofu loads .tf.json exactly as it loads .tf. This validator
		// parses native HCL, so a JSON file would sail past it and apply
		// whatever it liked. Refuse rather than grow a second parser: no
		// Layer 3 stack has ever needed one.
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tf.json") {
			return fmt.Errorf("layer 3 refuses %s: .tf.json is loaded by tofu but not validated here", entry.Name())
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}
		path := filepath.Join(outputDir, entry.Name())
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", entry.Name(), readErr)
		}
		file, diags := hclsyntax.ParseConfig(src, entry.Name(), hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			// Unparseable means unknowable. Fail closed rather than apply it.
			return fmt.Errorf("layer 3 refuses %s: cannot parse it, so cannot vouch for it (%s)", entry.Name(), diags.Error())
		}
		body, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			return fmt.Errorf("layer 3 refuses %s: unexpected body type", entry.Name())
		}
		fileProblems, sawProvider := layer3BlockProblems(body, entry.Name(), allowedResourceTypes)
		problems = append(problems, fileProblems...)
		sawCanonicalProvider = sawCanonicalProvider || sawProvider
	}
	if !sawCanonicalProvider {
		// Omitting required_providers is not a safe default: tofu then
		// resolves scaleway_* from the default namespace, which is a
		// choice the configuration made implicitly rather than one this
		// check verified. Declaring it is the only way to know which
		// provider binary will run beside the credentials.
		problems = append(problems, fmt.Sprintf("no terraform.required_providers entry declares source %q, so the provider binary would be resolved implicitly", layer3ScalewayProviderSource))
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("layer 3 refuses this configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}

func layer3BlockProblems(body *hclsyntax.Body, file string, allowedResourceTypes []string) ([]string, bool) {
	problems := make([]string, 0)
	sawCanonicalProvider := false
	for _, block := range body.Blocks {
		switch {
		case !layer3AllowedTopLevelBlocks[block.Type]:
			problems = append(problems, fmt.Sprintf("%s: %q blocks are not permitted", file, block.Type))
			continue
		case block.Type == "resource":
			if len(block.Labels) > 0 && !resourceTypeAllowed(block.Labels[0], allowedResourceTypes) {
				problems = append(problems, fmt.Sprintf("%s: resource type %q is not in allow_resource_types", file, block.Labels[0]))
			}
			problems = append(problems, layer3ProjectBindingProblems(block, file)...)
		case block.Type == "data":
			// `data "external"` runs a program at plan time. Restricting
			// data sources to the cloud under test removes that whole
			// family rather than naming the dangerous ones.
			if len(block.Labels) > 0 && !strings.HasPrefix(block.Labels[0], "scaleway_") {
				problems = append(problems, fmt.Sprintf("%s: data source %q is not a scaleway_ source", file, block.Labels[0]))
			}
		case block.Type == "provider":
			if len(block.Labels) > 0 && block.Labels[0] != "scaleway" {
				problems = append(problems, fmt.Sprintf("%s: provider %q is not permitted", file, block.Labels[0]))
				break
			}
			if block.Body != nil {
				for name := range block.Body.Attributes {
					if !layer3SafeProviderAttrs[name] {
						problems = append(problems, fmt.Sprintf("%s: provider setting %q is not permitted (the endpoint and project come from the sealed environment, not from the configuration)", file, name))
					}
				}
			}
		}
		if block.Type == "terraform" {
			tfProblems, sawProvider := layer3ProviderSourceProblems(block, file)
			problems = append(problems, tfProblems...)
			sawCanonicalProvider = sawCanonicalProvider || sawProvider
		}
		problems = append(problems, layer3NestedProblems(block, file)...)
	}
	return problems, sawCanonicalProvider
}

// layer3NestedProblems walks the whole tree: a provisioner is nested inside
// a resource, not top level.
func layer3NestedProblems(block *hclsyntax.Block, file string) []string {
	problems := make([]string, 0)
	if block.Body == nil {
		return problems
	}
	for _, nested := range block.Body.Blocks {
		if layer3DeniedNestedBlocks[nested.Type] {
			problems = append(problems, fmt.Sprintf("%s: %q executes commands during apply, in a process holding cloud credentials", file, nested.Type))
		}
		problems = append(problems, layer3NestedProblems(nested, file)...)
	}
	return problems
}

// layer3ScalewayProviderSource is the only provider a Layer 3 stack may pull.
const layer3ScalewayProviderSource = "scaleway/scaleway"

// layer3ProviderSourceProblems refuses any required_providers entry that is
// not the real Scaleway provider.
//
// Allowing `terraform` blocks wholesale left a hole that the block-type
// allowlist could not see: a fixture can write
//
//	terraform { required_providers { scaleway = { source = "attacker/scaleway" } } }
//
// and the local name still satisfies the `provider "scaleway"` check while
// tofu init downloads and EXECUTES that plugin with SCW_ACCESS_KEY and
// SCW_SECRET_KEY in the environment. The provider binary is code, and this
// path takes it from a registry address supplied by a pull request.
func layer3ProviderSourceProblems(tfBlock *hclsyntax.Block, file string) ([]string, bool) {
	problems := make([]string, 0)
	sawCanonical := false
	if tfBlock.Body == nil {
		return problems, sawCanonical
	}
	for _, inner := range tfBlock.Body.Blocks {
		if inner.Type != "required_providers" || inner.Body == nil {
			continue
		}
		for name, attr := range inner.Body.Attributes {
			val, diags := attr.Expr.Value(nil)
			if diags.HasErrors() || val.IsNull() || !val.Type().IsObjectType() {
				problems = append(problems, fmt.Sprintf("%s: required_provider %q is not a literal object this check can verify", file, name))
				continue
			}
			if !val.Type().HasAttribute("source") {
				problems = append(problems, fmt.Sprintf("%s: required_provider %q declares no source", file, name))
				continue
			}
			src := val.GetAttr("source")
			if src.IsNull() || src.Type() != cty.String || src.AsString() != layer3ScalewayProviderSource {
				problems = append(problems, fmt.Sprintf("%s: required_provider %q must be source %q (a provider binary is code, and this one is chosen by the PR)",
					file, name, layer3ScalewayProviderSource))
				continue
			}
			// Only the `scaleway` LOCAL NAME satisfies the requirement.
			// `foo = { source = "scaleway/scaleway" }` declares a correct
			// source under a name nothing uses, while scaleway_* resources
			// still resolve `scaleway` implicitly -- which is the exact
			// case this check exists to catch.
			if name == "scaleway" {
				sawCanonical = true
			}
		}
	}
	return problems, sawCanonical
}

// layer3ProjectBindingProblems refuses a project_id that is not a reference
// to the stack's own scaleway_account_project.
//
// The whole blast-radius argument (ADR-0010, ADR-0023 rule 4) is that each
// run creates a disposable project and everything lives inside it. Checking
// that a scaleway_account_project EXISTS is not enough: a fixture can
// declare one to satisfy that check and then pin its actual resources
// elsewhere with `project_id = "<some other project>"`. On this account
// "elsewhere" includes the project holding live infrastructure.
//
// A literal, a variable, or a data lookup are all refused. Only a direct
// reference to a scaleway_account_project resource in this stack is
// accepted, because only that is provably the project the sweep will
// destroy.
func layer3ProjectBindingProblems(resource *hclsyntax.Block, file string) []string {
	problems := make([]string, 0)
	if resource.Body == nil {
		return problems
	}
	attr, ok := resource.Body.Attributes["project_id"]
	if !ok {
		return problems
	}
	name := "<unnamed>"
	if len(resource.Labels) > 1 {
		name = resource.Labels[1]
	}
	vars := attr.Expr.Variables()
	if len(vars) == 0 {
		problems = append(problems, fmt.Sprintf("%s: %s sets project_id to a literal; it must reference this stack's scaleway_account_project, which is the only project the sweep will destroy", file, name))
		return problems
	}
	for _, traversal := range vars {
		root := traversal.RootName()
		if root != "scaleway_account_project" {
			problems = append(problems, fmt.Sprintf("%s: %s sets project_id from %q; it must reference this stack's scaleway_account_project", file, name, root))
		}
	}
	return problems
}

// layer3PreflightHCL is every structural check Layer 3 makes on a
// configuration, in the order that matters: all of them before any tofu
// process starts.
func layer3PreflightHCL(outputDir string, allowedResourceTypes []string) error {
	if err := validateLayer3ProjectResource(outputDir); err != nil {
		return err
	}
	return validateLayer3HCLShape(outputDir, allowedResourceTypes)
}
