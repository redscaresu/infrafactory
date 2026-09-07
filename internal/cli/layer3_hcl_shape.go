package cli

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
// Every parent reference is listed, not just the one that scopes
// billing. A private NIC names both a server and a private network, and
// checking only the server lets a literal private_network_id attach the
// run's server to a network the run does not own and will not sweep.
// Same for a frontend, which names a backend as well as a load balancer.
var layer3ChildScopedTypes = map[string][]string{
	"scaleway_lb_backend":           {"lb_id"},
	"scaleway_lb_frontend":          {"lb_id", "backend_id"},
	"scaleway_lb_route":             {"frontend_id", "backend_id"},
	"scaleway_lb_certificate":       {"lb_id"},
	"scaleway_instance_private_nic": {"server_id", "private_network_id"},
}

// layer3ProjectExemptTypes carry no project binding of any kind.
//
// scaleway_account_project used to be here because it IS the project.
// Under ADR-0025 it is refused outright instead: the run's project is
// created before the apply, so a stack that declares one would create a
// SECOND project that nothing tracks and no teardown would delete.
var layer3ProjectExemptTypes = map[string]bool{}

// layer3UnreadableConfigExt reports whether tofu would load the file
// automatically while validateLayer3HCLShape would not read it.
//
// Two families, both auto-loaded and neither parsed here:
//
//   - .tf.json, .tofu, .tofu.json -- configuration in a dialect this
//     validator does not speak.
//   - terraform.tfvars and *.auto.tfvars (and their .json forms) --
//     variable VALUES, loaded with no flag. The .tf files can validate
//     perfectly and still apply something else: `size_in_gb = var.size`
//     is checked once and priced by whatever the tfvars says.
//
// Checked by suffix, longest-first: ".tf.json" also ends in ".json", and
// a plain ".tf" must stay readable.
// ErrLayer3RefusesConfiguration marks a preflight refusal whose text is
// SAFE TO SHOW and worth showing.
//
// Every problem in that message is composed here, names a file by its
// base name, and says exactly what to remove -- "main.tf: scaleway_lb
// main sets project_id ... Remove the attribute". It is the most
// actionable error on the deploy path.
//
// It needs a sentinel because the deploy path now withholds an error's
// text by default, which was right for `*fs.PathError` and wrong for
// this: a real run refused here and the operator was told only "see the
// server log", for a fault they could have fixed in thirty seconds from
// the message the server was hiding.

var ErrLayer3RefusesConfiguration = errors.New("layer 3 refuses this configuration")

func layer3UnreadableConfigExt(name string) bool {
	if name == "terraform.tfvars" || name == "terraform.tfvars.json" {
		return true
	}
	for _, ext := range []string{".tf.json", ".tofu.json", ".tofu", ".auto.tfvars", ".auto.tfvars.json"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
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
	projectResources := 0
	// Variable defaults are collected across ALL files before any
	// resource is checked, because a bound attribute in main.tf routinely
	// reads a default declared in variables.tf. A single pass would see
	// `size_in_gb = var.volume_size_in_gb` with no default in scope yet,
	// call it unresolvable, and refuse a committed fixture.
	parsed := make(map[string]*hclsyntax.Body)
	varDefaults := make(map[string]cty.Value)
	for _, entry := range entries {
		// Every extension tofu loads must be one this validator reads.
		//
		// .tf.json is loaded exactly as .tf and would sail past a native
		// HCL parser. OpenTofu also loads .tofu and .tofu.json, which are
		// easier still to miss -- a .tofu file sitting beside valid .tf
		// gets applied with cloud credentials having passed none of the
		// checks below. Refuse rather than grow a second parser and a
		// second dialect: no Layer 3 stack has ever needed either.
		if !entry.IsDir() && layer3UnreadableConfigExt(entry.Name()) {
			return fmt.Errorf("layer 3 refuses %s: tofu loads it automatically but this validator does not read it", entry.Name())
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
		parsed[entry.Name()] = body
		layer3VariableDefaults(body, varDefaults)
	}
	for _, name := range slices.Sorted(maps.Keys(parsed)) {
		body := parsed[name]
		fileProblems, sawProvider, projectsHere := layer3BlockProblems(body, name, allowedResourceTypes)
		problems = append(problems, fileProblems...)
		sawCanonicalProvider = sawCanonicalProvider || sawProvider
		projectResources += projectsHere
		for _, block := range body.Blocks {
			if block.Type == "resource" {
				problems = append(problems, layer3CostProblems(block, name, varDefaults)...)
			}
		}
	}
	// The teardown model assumes a single disposable project throughout:
	// the marker records one project id, AssertRunProjectDeletable guards
	// that one, and the orphan sweep asks the API about that one.
	// Inverted by ADR-0025: the run's disposable project is created before
	// the apply and handed to the provider, so a stack must declare NONE.
	// Declaring one creates a second project that nothing tracks, no
	// marker names and no teardown deletes -- which is the leak the
	// single-project rule was written to prevent, arrived at from the
	// other direction.
	if projectResources > 0 {
		problems = append(problems, fmt.Sprintf(
			"%d scaleway_account_project resource(s) declared; the run's project is created before the apply and is the provider default, so a declared one would be a second project nothing tracks or destroys",
			projectResources))
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
		return fmt.Errorf("%w: %s", ErrLayer3RefusesConfiguration, strings.Join(problems, "; "))
	}
	return nil
}

func layer3BlockProblems(body *hclsyntax.Body, file string, allowedResourceTypes []string) ([]string, bool, int) {
	problems := make([]string, 0)
	sawCanonicalProvider := false
	projectResources := 0
	for _, block := range body.Blocks {
		switch {
		case !layer3AllowedTopLevelBlocks[block.Type]:
			problems = append(problems, fmt.Sprintf("%s: %q blocks are not permitted", file, block.Type))
			continue
		case block.Type == "resource":
			if len(block.Labels) > 0 && block.Labels[0] == "scaleway_account_project" {
				projectResources++
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
		problems = append(problems, layer3UndestroyableProblems(block, file)...)
	}
	return problems, sawCanonicalProvider, projectResources
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

	if resourceType == "scaleway_account_project" {
		return append(problems, fmt.Sprintf(
			"%s: %s declares a %s; under ADR-0025 the run's project is created before the apply and handed to the provider, so a stack that declares one would create a SECOND project that nothing tracks and no teardown would delete. Remove it",
			file, name, resourceType))
	}

	if layer3ProjectExemptTypes[resourceType] {
		return problems
	}

	if parentAttrs, isChild := layer3ChildScopedTypes[resourceType]; isChild {
		for _, parentAttr := range parentAttrs {
			problems = append(problems, layer3ParentBindingProblems(resource, file, name, parentAttr)...)
		}
		return problems
	}

	// ADR-0025 inverts this rule. The run's project is created before the
	// apply and handed to the provider as SCW_DEFAULT_PROJECT_ID, so a
	// resource that sets NO project_id lands in it by construction --
	// which is what makes scaleway_instance_private_nic, an attribute
	// that cannot carry one, applicable at all.
	//
	// So project_id is now FORBIDDEN rather than required. Allowing it
	// would let a configuration name some other project and escape the
	// run's blast radius, which is exactly what the old binding rule
	// existed to stop. Forbidding it is the stronger form of the same
	// guarantee: there is no value to get wrong.
	if _, hasProject := resource.Body.Attributes["project_id"]; hasProject {
		return append(problems, fmt.Sprintf(
			"%s: %s %s sets project_id; under ADR-0025 the run's project is the provider default, so a resource that names a project could place itself outside this run's blast radius. Remove the attribute",
			file, resourceType, name))
	}
	return problems
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
// Allowlisted rather than denylisted, and the allowlist holds only PURE
// functions -- ones that compute over their arguments and touch nothing
// else. The dangerous families all share one property: they reach
// outside the expression, into the filesystem (file, templatefile,
// fileset, filebase64) or the runner's paths (abspath, pathexpand). A
// denylist of those names would be another enumerate-the-bad-ones list,
// and every one of those on this surface has produced a bypass.
//
// A blanket ban was the first attempt and it was too strict to survive
// contact with the generator, which writes
// `tags = concat(var.tags, ["web-server"])`. concat cannot read a
// secret. Refusing it bought nothing and broke Layer 3 for every
// generated stack.
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
			if layer3PureFunctions[fn] {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"%s: %s calls %s(), which is not on the pure-function allowlist; Layer 3 evaluates this HCL with real credentials in the environment, and a function that reads the filesystem can put them in a resource attribute",
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

// layer3PureFunctions compute over their arguments and reach nothing
// else -- no filesystem, no runner paths, no external process. Adding to
// this list is a deliberate act: the question to ask of a candidate is
// not "is it useful?" but "can its result depend on anything outside its
// arguments?"
var layer3PureFunctions = map[string]bool{
	// `try` is already below; `one` is added because it is the other way
	// an index is made total, and refusing it would refuse the fix this
	// preflight recommends.
	"one": true,

	// collections
	"concat": true, "merge": true, "lookup": true, "element": true,
	"length": true, "keys": true, "values": true, "flatten": true,
	"distinct": true, "compact": true, "slice": true, "contains": true,
	"coalesce": true, "coalescelist": true, "zipmap": true, "range": true,
	"tolist": true, "toset": true, "tomap": true,
	// strings
	"format": true, "formatlist": true, "join": true, "split": true,
	"replace": true, "substr": true, "trimspace": true, "trim": true,
	"trimprefix": true, "trimsuffix": true, "lower": true, "upper": true,
	"title": true, "chomp": true, "regex": true, "regexall": true,
	// numbers and types
	"max": true, "min": true, "abs": true, "ceil": true, "floor": true,
	"tostring": true, "tonumber": true, "tobool": true,
	"jsonencode": true, "jsondecode": true, "try": true, "can": true,
	// networking, used by generated VPC scenarios
	"cidrsubnet": true, "cidrhost": true, "cidrnetmask": true,
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

// layer3AttributeBounds caps the cost-bearing attributes of the allowed
// resource types.
//
// The allowlist bounds which TYPES may be created and says nothing about
// how expensive an instance of one is. `scaleway_block_volume` stays
// within it at 10 GB and at 10 TB; `scaleway_instance_server` stays
// within it at DEV1-S and at the largest GPU node Scaleway rents. The
// spend ceiling for this whole arc is single-digit pounds.
//
// Numeric entries are maxima. String entries are exact permitted values.
// Both are deliberately close to what the committed fixtures use: this
// is a demo gate applying a handful of small resources, not a general
// deployment pipeline, and a fixture that needs more should say so in a
// base-branch change.
var layer3NumericBounds = map[string]map[string]float64{
	"scaleway_block_volume": {
		"size_in_gb": 20,
		"iops":       15000,
	},
}

var layer3EnumBounds = map[string]map[string][]string{
	"scaleway_instance_server": {"type": {"DEV1-S", "DEV1-M"}},
	"scaleway_lb":              {"type": {"LB-S"}},
}

// layer3CostProblems refuses out-of-bound cost attributes.
//
// Values reach the attribute either literally or through a variable
// default -- tfvars files are refused outright, and function calls are
// too, so those are the only two forms left. An attribute whose value
// cannot be resolved to a constant is refused rather than assumed small.
func layer3CostProblems(resource *hclsyntax.Block, file string, varDefaults map[string]cty.Value) []string {
	problems := make([]string, 0)
	if resource.Body == nil || len(resource.Labels) == 0 {
		return problems
	}
	resourceType := resource.Labels[0]
	name := "<unnamed>"
	if len(resource.Labels) > 1 {
		name = resource.Labels[1]
	}

	for attrName, max := range layer3NumericBounds[resourceType] {
		attr, ok := resource.Body.Attributes[attrName]
		if !ok {
			continue
		}
		val, resolved := layer3ResolveConstant(attr.Expr, varDefaults)
		if !resolved || val.Type() != cty.Number {
			problems = append(problems, fmt.Sprintf("%s: %s sets %s to something this check cannot resolve to a constant, so its cost cannot be bounded", file, name, attrName))
			continue
		}
		f, _ := val.AsBigFloat().Float64()
		if f > max {
			problems = append(problems, fmt.Sprintf("%s: %s sets %s to %g; the gate caps it at %g because it applies to real, billed infrastructure", file, name, attrName, f, max))
		}
	}

	// Nested blocks carry cost too: scaleway_instance_server declares its
	// disk as `root_volume { size_in_gb = ... }`, so a top-level-only
	// check bounds the server type and lets the disk be any size.
	for _, inner := range resource.Body.Blocks {
		problems = append(problems, layer3NestedCostProblems(inner, file, name, varDefaults)...)
	}

	for attrName, allowed := range layer3EnumBounds[resourceType] {
		attr, ok := resource.Body.Attributes[attrName]
		if !ok {
			continue
		}
		val, resolved := layer3ResolveConstant(attr.Expr, varDefaults)
		if !resolved || val.Type() != cty.String {
			problems = append(problems, fmt.Sprintf("%s: %s sets %s to something this check cannot resolve to a constant, so its cost cannot be bounded", file, name, attrName))
			continue
		}
		if !slices.Contains(allowed, val.AsString()) {
			problems = append(problems, fmt.Sprintf("%s: %s sets %s to %q; the gate permits only %v", file, name, attrName, val.AsString(), allowed))
		}
	}
	return problems
}

// layer3ResolveConstant evaluates a literal, or a `var.x` whose default
// is a literal. Anything else is unresolved -- and unresolved is refused
// by the caller, never assumed harmless.
func layer3ResolveConstant(expr hclsyntax.Expression, varDefaults map[string]cty.Value) (cty.Value, bool) {
	if traversal, ok := expr.(*hclsyntax.ScopeTraversalExpr); ok {
		if traversal.Traversal.RootName() != "var" || len(traversal.Traversal) < 2 {
			return cty.NilVal, false
		}
		attr, ok := traversal.Traversal[1].(hcl.TraverseAttr)
		if !ok {
			return cty.NilVal, false
		}
		val, ok := varDefaults[attr.Name]
		return val, ok
	}
	val, diags := expr.Value(nil)
	if diags.HasErrors() || val.IsNull() {
		return cty.NilVal, false
	}
	return val, true
}

// layer3VariableDefaults collects `variable "x" { default = <literal> }`.
func layer3VariableDefaults(body *hclsyntax.Body, into map[string]cty.Value) {
	for _, block := range body.Blocks {
		if block.Type != "variable" || len(block.Labels) == 0 || block.Body == nil {
			continue
		}
		attr, ok := block.Body.Attributes["default"]
		if !ok {
			continue
		}
		if val, diags := attr.Expr.Value(nil); !diags.HasErrors() && !val.IsNull() {
			into[block.Labels[0]] = val
		}
	}
}

// layer3NestedCostBounds caps attributes wherever they appear inside a
// resource, regardless of which block holds them. Keyed by attribute
// rather than by block, because the same name means the same thing --
// and enumerating every nested block Scaleway might add is the kind of
// list this surface keeps proving wrong.
var layer3NestedCostBounds = map[string]float64{
	"size_in_gb": 20,
	"iops":       15000,
}

func layer3NestedCostProblems(block *hclsyntax.Block, file, owner string, varDefaults map[string]cty.Value) []string {
	problems := make([]string, 0)
	if block.Body == nil {
		return problems
	}
	for attrName, max := range layer3NestedCostBounds {
		attr, ok := block.Body.Attributes[attrName]
		if !ok {
			continue
		}
		val, resolved := layer3ResolveConstant(attr.Expr, varDefaults)
		if !resolved || val.Type() != cty.Number {
			problems = append(problems, fmt.Sprintf("%s: %s sets %s.%s to something this check cannot resolve to a constant, so its cost cannot be bounded", file, owner, block.Type, attrName))
			continue
		}
		f, _ := val.AsBigFloat().Float64()
		if f > max {
			problems = append(problems, fmt.Sprintf("%s: %s sets %s.%s to %g; the gate caps it at %g because it applies to real, billed infrastructure", file, owner, block.Type, attrName, f, max))
		}
	}
	for _, inner := range block.Body.Blocks {
		problems = append(problems, layer3NestedCostProblems(inner, file, owner, varDefaults)...)
	}
	return problems
}

// layer3UndestroyableProblems refuses an index into a resource attribute
// that is not made total by try() or one().
//
// # This is a teardown check, not an apply check
//
// `tofu destroy` EVALUATES THE CONFIGURATION. It is not a replay of
// state. So an expression that cannot be evaluated does not merely fail
// the apply -- it disables the escape hatch, and the infrastructure the
// half-finished apply created cannot be removed by the tool that created
// it.
//
// Found by running it (S164, 2026-09-07). `web-live-paris` indexed
// `scaleway_instance_private_nic.web.private_ips[0].address`; the list
// was empty at apply time, so apply failed with "Invalid index" --
// and then teardown failed with the SAME error, and `run_project_delete`
// could not rescue it either, because Scaleway refuses to delete a
// project that still holds resources. Recovery meant hand-editing the
// workdir's HCL and running `tofu destroy` by hand. An operator without
// terraform and shell access would have had stranded, billing
// infrastructure and a red banner.
//
// ADR-0024 held -- teardown correctly refused to claim success -- but
// there was no path back to a clean account inside the product. So the
// defect to prevent is not "apply failed"; it is "we let a stack reach
// apply that we could not have destroyed", and that is checkable here,
// before anything is created.
//
// # Why resource attributes specifically
//
// `var.x[0]` and `local.y[0]` are known at plan time and cannot surprise
// a destroy. A resource attribute can be unknown or empty until after
// apply, which is exactly the case that strands things. Indexing one is
// refused unless it is wrapped: `try(...)` and `one(...)` both make the
// expression total, so destroy can always evaluate it.
func layer3UndestroyableProblems(block *hclsyntax.Block, file string) []string {
	problems := make([]string, 0)
	if block.Body == nil {
		return problems
	}
	for name, attr := range block.Body.Attributes {
		for _, ref := range layer3UnguardedResourceIndexes(attr.Expr) {
			problems = append(problems, fmt.Sprintf(
				"%s: %s indexes %s, and a resource attribute can be empty until after apply; `tofu destroy` evaluates the configuration, so if this index fails the stack cannot be destroyed either. Wrap it in try() or one()",
				file, name, ref))
		}
	}
	for _, inner := range block.Body.Blocks {
		problems = append(problems, layer3UndestroyableProblems(inner, file)...)
	}
	return problems
}

func layer3UnguardedResourceIndexes(expr hclsyntax.Expression) []string {
	var found []string
	guard := 0
	// Both fields are pointers because hclsyntax.Walk takes the walker by
	// VALUE and calls Enter/Exit on copies. A lazily-initialised counter
	// inside Enter silently reset on every node.
	_ = hclsyntax.Walk(expr, layer3IndexWalker{found: &found, guard: &guard})
	return found
}

// layer3IndexWalker collects indexes into resource attributes, skipping
// anything already inside a total-making call.
type layer3IndexWalker struct {
	found *[]string
	// guard counts enclosing try()/one() calls. Walk is depth-first with
	// matched Enter/Exit, so a counter tracks "am I inside one" without
	// carrying a stack.
	guard *int
}

func (w layer3IndexWalker) Enter(node hclsyntax.Node) hcl.Diagnostics {
	if call, ok := node.(*hclsyntax.FunctionCallExpr); ok && layer3TotalisingFunctions[call.Name] {
		*w.guard++
		return nil
	}
	if *w.guard > 0 {
		return nil
	}
	// A TRAVERSAL step, not an IndexExpr.
	//
	// `a.b.c[0].d` is one ScopeTraversalExpr whose Traversal holds
	// TraverseRoot, TraverseAttr, TraverseIndex, TraverseAttr -- there is
	// no IndexExpr node anywhere in it. Looking for IndexExpr found
	// nothing at all and the check passed everything, which is how the
	// first version of this reported clean on the exact HCL that stranded
	// a stack.
	//
	// IndexExpr does exist for the `x[expr]` form on a non-traversal, so
	// both are handled.
	switch e := node.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		if ref := layer3IndexedResourceIn(e.Traversal); ref != "" {
			*w.found = append(*w.found, ref)
		}
	case *hclsyntax.RelativeTraversalExpr:
		if ref := layer3IndexedResourceIn(e.Traversal); ref != "" {
			*w.found = append(*w.found, ref)
		}
	case *hclsyntax.IndexExpr:
		if ref := layer3ResourceReference(e.Collection); ref != "" {
			*w.found = append(*w.found, ref)
		}
	}
	return nil
}

// layer3IndexedResourceIn reports the resource attribute an indexed
// traversal reaches into, or "".
func layer3IndexedResourceIn(traversal hcl.Traversal) string {
	if len(traversal) < 2 {
		return ""
	}
	root, ok := traversal[0].(hcl.TraverseRoot)
	if !ok {
		return ""
	}
	switch root.Name {
	case "var", "local", "each", "count", "path", "terraform":
		return ""
	}
	name := root.Name
	for _, step := range traversal[1:] {
		switch t := step.(type) {
		case hcl.TraverseAttr:
			name += "." + t.Name
		case hcl.TraverseIndex:
			// The index is what makes it a hazard; everything before it
			// is the attribute being indexed.
			return name
		}
	}
	return ""
}

func (w layer3IndexWalker) Exit(node hclsyntax.Node) hcl.Diagnostics {
	if call, ok := node.(*hclsyntax.FunctionCallExpr); ok && layer3TotalisingFunctions[call.Name] && *w.guard > 0 {
		*w.guard--
	}
	return nil
}

// layer3TotalisingFunctions make an expression evaluable whatever the
// collection holds, which is the whole property destroy needs.
var layer3TotalisingFunctions = map[string]bool{
	"try": true,
	"one": true,
}

// layer3ResourceReference renders a traversal that names a RESOURCE
// attribute, or "" for anything else.
//
// `var.`, `local.`, `each.` and `count.` are excluded deliberately: they
// are known before apply, so indexing them cannot strand a stack.
func layer3ResourceReference(expr hclsyntax.Expression) string {
	scope, ok := expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok || len(scope.Traversal) < 2 {
		return ""
	}
	root, ok := scope.Traversal[0].(hcl.TraverseRoot)
	if !ok {
		return ""
	}
	switch root.Name {
	case "var", "local", "each", "count", "path", "terraform":
		return ""
	}
	parts := root.Name
	for _, step := range scope.Traversal[1:] {
		attr, ok := step.(hcl.TraverseAttr)
		if !ok {
			break
		}
		parts += "." + attr.Name
	}
	return parts
}
