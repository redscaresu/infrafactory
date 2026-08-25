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
// layer3AllowedTopLevelBlocks is deny-by-default. `data` is deliberately
// absent: a data source reads the REAL account, outside the run's
// disposable project, and its result can be interpolated into an allowed
// resource -- `user_data` on an instance the PR boot-scripts, or an
// output the gate posts onto the pull request. Refusing function calls
// closed the file() path; a data block is the same exfiltration with a
// traversal instead of a call.
//
// No gate fixture uses one. If a scenario ever needs a data source, it
// gets admitted deliberately and with a story for what it may read.
var layer3AllowedTopLevelBlocks = map[string]bool{
	"terraform": true,
	"provider":  true,
	"variable":  true,
	"output":    true,
	"locals":    true,
	"resource":  true,
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

// layer3ChildScopedTypes are resource types that carry no project_id
// because they live inside a parent resource. The value is the attribute
// naming that parent.
//
// Their containment is inherited: an lb_backend belongs to whichever load
// balancer its lb_id names, and that load balancer is itself project-bound.
// That only holds if the parent is a resource in THIS stack --
// `lb_id = "<some existing lb uuid>"` would attach a backend to a load
// balancer the run does not own and will never destroy.
var layer3ChildScopedTypes = map[string]string{
	"scaleway_lb_backend":           "lb_id",
	"scaleway_lb_frontend":          "lb_id",
	"scaleway_lb_route":             "frontend_id",
	"scaleway_lb_certificate":       "lb_id",
	"scaleway_instance_private_nic": "server_id",
}

// layer3ProjectExemptTypes carry no project binding of any kind.
// scaleway_account_project IS the project.
var layer3ProjectExemptTypes = map[string]bool{
	"scaleway_account_project": true,
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
	sawProjectResource := false
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
		fileProblems, sawProvider, sawProject := layer3BlockProblems(body, entry.Name(), allowedResourceTypes)
		problems = append(problems, fileProblems...)
		sawCanonicalProvider = sawCanonicalProvider || sawProvider
		sawProjectResource = sawProjectResource || sawProject
	}
	if !sawProjectResource {
		problems = append(problems, "no scaleway_account_project resource is declared, so this stack has no disposable project of its own to create and destroy")
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

func layer3BlockProblems(body *hclsyntax.Body, file string, allowedResourceTypes []string) ([]string, bool, bool) {
	problems := make([]string, 0)
	sawCanonicalProvider := false
	sawProjectResource := false
	for _, block := range body.Blocks {
		switch {
		case !layer3AllowedTopLevelBlocks[block.Type]:
			problems = append(problems, fmt.Sprintf("%s: %q blocks are not permitted", file, block.Type))
			continue
		case block.Type == "resource":
			if len(block.Labels) > 0 && block.Labels[0] == "scaleway_account_project" {
				sawProjectResource = true
			}
			if len(block.Labels) > 0 && !resourceTypeAllowed(block.Labels[0], allowedResourceTypes) {
				problems = append(problems, fmt.Sprintf("%s: resource type %q is not in allow_resource_types", file, block.Labels[0]))
			}
			problems = append(problems, layer3ContainmentProblems(block, file)...)
			problems = append(problems, layer3MultiplicityProblems(block, file)...)
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
		problems = append(problems, layer3FunctionCallProblems(block, file)...)
	}
	return problems, sawCanonicalProvider, sawProjectResource
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

// layer3ScalewayProviderVersion is the exact provider version a Layer 3
// stack may pull, and it lives here -- in the binary the gate builds from
// the BASE branch -- precisely so a pull request cannot change it.
//
// A constraint is not a pin. The gate fixtures carried `~> 2.57` and
// resolved to 2.81.0, because a range means "whatever the registry is
// serving when init runs". The provider is an executable that runs with
// SCW_ACCESS_KEY and SCW_SECRET_KEY in its environment, so which build of
// it executes should be a decision someone made, not a decision the
// registry makes at 3am.
//
// Bumping this is a base-branch change, reviewed like any other.
const layer3ScalewayProviderVersion = "2.81.0"

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
// layer3ProviderVersionProblem requires an exact version equal to the
// trusted pin. Ranges, omissions and any other exact version are refused.
func layer3ProviderVersionProblem(val cty.Value, file, name string) (string, bool) {
	if !val.Type().HasAttribute("version") {
		return fmt.Sprintf("%s: required_provider %q declares no version; it must pin exactly %q, or tofu init downloads whatever the registry is serving",
			file, name, layer3ScalewayProviderVersion), false
	}
	version := val.GetAttr("version")
	if version.IsNull() || version.Type() != cty.String {
		return fmt.Sprintf("%s: required_provider %q has a version this check cannot read", file, name), false
	}
	if version.AsString() != layer3ScalewayProviderVersion {
		return fmt.Sprintf("%s: required_provider %q pins version %q; the gate only runs provider %q. A range such as \"~> 2.57\" is not a pin -- it resolves to whatever the registry serves at init time",
			file, name, version.AsString(), layer3ScalewayProviderVersion), false
	}
	return "", true
}

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
			if versionProblem, ok := layer3ProviderVersionProblem(val, file, name); !ok {
				problems = append(problems, versionProblem)
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
// layer3ContainmentProblems requires every resource to be provably inside
// the run's disposable project -- by binding project_id to it, or by
// belonging to a parent resource in this stack that is.
//
// Omitting project_id entirely was the gap this closes. The provider
// falls back to SCW_DEFAULT_PROJECT_ID, which the sealed environment
// points at the configured fallback project, so an allowed resource with
// no project_id applied cleanly to a REAL project the run never created
// and the sweep never destroys. Nothing in the HCL looked wrong.
func layer3ContainmentProblems(resource *hclsyntax.Block, file string) []string {
	problems := make([]string, 0)
	if resource.Body == nil || len(resource.Labels) == 0 {
		return problems
	}
	resourceType := resource.Labels[0]
	name := "<unnamed>"
	if len(resource.Labels) > 1 {
		name = resource.Labels[1]
	}

	if layer3ProjectExemptTypes[resourceType] {
		return problems
	}

	if parentAttr, isChild := layer3ChildScopedTypes[resourceType]; isChild {
		return layer3ParentBindingProblems(resource, file, name, parentAttr)
	}

	if _, hasProject := resource.Body.Attributes["project_id"]; !hasProject {
		return append(problems, fmt.Sprintf(
			"%s: %s %s sets no project_id, so it would be created in the fallback project rather than this run's disposable one; bind it to scaleway_account_project.<name>.id",
			file, resourceType, name))
	}
	return layer3ProjectBindingProblems(resource, file)
}

// layer3ParentBindingProblems requires a child resource's parent id to be
// a reference to a resource in this stack, for the same reason project_id
// must be: a literal UUID names infrastructure the run does not own.
func layer3ParentBindingProblems(resource *hclsyntax.Block, file, name, parentAttr string) []string {
	problems := make([]string, 0)
	attr, ok := resource.Body.Attributes[parentAttr]
	if !ok {
		return append(problems, fmt.Sprintf(
			"%s: %s sets no %s, so nothing ties it to a resource this run created", file, name, parentAttr))
	}
	traversal, ok := attr.Expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok || !strings.HasPrefix(traversal.Traversal.RootName(), "scaleway_") {
		return append(problems, fmt.Sprintf(
			"%s: %s must set %s to a reference to a resource in this stack; a literal id names infrastructure the run does not own and will not destroy",
			file, name, parentAttr))
	}
	last, isAttr := traversal.Traversal[len(traversal.Traversal)-1].(hcl.TraverseAttr)
	if len(traversal.Traversal) < 3 || !isAttr || last.Name != "id" {
		problems = append(problems, fmt.Sprintf(
			"%s: %s must set %s to <resource>.<name>.id", file, name, parentAttr))
	}
	return problems
}

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
	// The expression must BE the reference, not merely contain one.
	// Expr.Variables() reports which traversals appear, not what the
	// expression evaluates to, so
	//
	//	project_id = scaleway_account_project.main.id != "" ? "prod" : "prod"
	//
	// mentions the disposable project and resolves to another one entirely.
	// Requiring a bare traversal removes the whole class rather than trying
	// to evaluate arbitrary expressions.
	traversal, ok := attr.Expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok {
		problems = append(problems, fmt.Sprintf("%s: %s sets project_id to an expression; it must be a direct reference to this stack's scaleway_account_project, which is the only project the sweep will destroy", file, name))
		return problems
	}
	if traversal.Traversal.RootName() != "scaleway_account_project" {
		problems = append(problems, fmt.Sprintf("%s: %s sets project_id from %q; it must reference this stack's scaleway_account_project", file, name, traversal.Traversal.RootName()))
		return problems
	}
	// ...and specifically its .id. `scaleway_account_project.main.name` is
	// also a reference to the project, but the NAME is chosen by the PR and
	// can be set to any existing project's UUID.
	last, ok := traversal.Traversal[len(traversal.Traversal)-1].(hcl.TraverseAttr)
	if len(traversal.Traversal) < 3 || !ok || last.Name != "id" {
		problems = append(problems, fmt.Sprintf("%s: %s must set project_id to scaleway_account_project.<name>.id; any other attribute is a value the PR chooses", file, name))
	}
	return problems
}

// layer3PreflightHCL is every structural check Layer 3 makes on a
// configuration, in the order that matters: all of them before any tofu
// process starts.
// The project check is done by the PARSER inside validateLayer3HCLShape,
// not by the substring scan used on the generation path: a comment or a
// string containing `resource "scaleway_account_project"` satisfies the
// latter while the stack declares no such resource, and an allowed
// resource would then land in the configured fallback project instead of
// a disposable one.
func layer3PreflightHCL(outputDir string, allowedResourceTypes []string) error {
	return validateLayer3HCLShape(outputDir, allowedResourceTypes)
}

// layer3FunctionCallProblems refuses every function call in PR-supplied
// HCL.
//
// Blocking provisioners, `data "external"` and the rest closed the paths
// that obviously execute something. Ordinary expressions were still
// unrestricted, and that is enough:
//
//	resource "scaleway_instance_server" "web" {
//	  user_data = { cloud-init = file("/proc/self/environ") }
//	}
//
// reads the runner's environment -- SCW_ACCESS_KEY and SCW_SECRET_KEY
// included -- and hands it to a machine the PR controls the boot script
// of. An `output` would do it more directly still, since the gate posts
// its output to the pull request.
//
// Deny-by-default rather than a denylist of file/templatefile/fileset,
// for the same reason the block check is: this surface has produced a
// bypass for every enumerate-the-bad-ones attempt on it. A fixture that
// genuinely needs a function can have one allowlisted, deliberately.
//
// Type constraints are exempt. `type = list(string)` parses as a call and
// is not one -- it names a type and evaluates nothing.
func layer3FunctionCallProblems(block *hclsyntax.Block, file string) []string {
	problems := make([]string, 0)
	if block.Body == nil {
		return problems
	}
	isVariable := block.Type == "variable"
	for name, attr := range block.Body.Attributes {
		if isVariable && name == "type" {
			continue
		}
		for _, fn := range layer3CallsIn(attr.Expr) {
			problems = append(problems, fmt.Sprintf(
				"%s: %s calls %s(); Layer 3 evaluates PR-supplied HCL with real credentials in the environment, so function calls are refused (file() and friends can read them, and the stack can ship them out)",
				file, name, fn))
		}
	}
	for _, inner := range block.Body.Blocks {
		problems = append(problems, layer3FunctionCallProblems(inner, file)...)
	}
	return problems
}

// layer3CallsIn reports the function names called anywhere inside an
// expression, including nested inside other calls and collections.
func layer3CallsIn(expr hclsyntax.Expression) []string {
	var found []string
	walker := layer3CallWalker{found: &found}
	_ = hclsyntax.Walk(expr, walker)
	return found
}

type layer3CallWalker struct{ found *[]string }

func (w layer3CallWalker) Enter(node hclsyntax.Node) hcl.Diagnostics {
	if call, ok := node.(*hclsyntax.FunctionCallExpr); ok {
		*w.found = append(*w.found, call.Name)
	}
	return nil
}

func (w layer3CallWalker) Exit(hclsyntax.Node) hcl.Diagnostics { return nil }

// layer3MultiplicityProblems refuses count and for_each on PR-supplied
// resources.
//
// Every other check here asks "may this resource type exist?" and none
// asked "how many?". `count = 50` on an allowed scaleway_instance_server
// is fifty real servers billing by the hour, and the arc's spend ceiling
// is not enforced by anything else -- ADR-0010 bounds the blast radius to
// one project, not the contents of it.
//
// The other direction matters as much and is quieter: `count = 0` makes
// an allowed resource vanish, so the gate applies nothing, probes
// nothing, sweeps clean and reports green. That is a false green, which
// is the failure this whole workflow exists to eliminate.
//
// A fixture that genuinely needs several of something can write them out.
// The gate applies a handful of resources by design.
func layer3MultiplicityProblems(resource *hclsyntax.Block, file string) []string {
	problems := make([]string, 0)
	if resource.Body == nil {
		return problems
	}
	name := "<unnamed>"
	if len(resource.Labels) > 1 {
		name = resource.Labels[1]
	}
	for _, meta := range []string{"count", "for_each"} {
		if _, ok := resource.Body.Attributes[meta]; ok {
			problems = append(problems, fmt.Sprintf(
				"%s: %s sets %s; Layer 3 applies to real infrastructure, so how MANY resources a fixture creates is not the PR's to choose (and %s = 0 would make the gate verify nothing and still report green)",
				file, name, meta, meta))
		}
	}
	return problems
}
