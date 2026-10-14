package cli

import (
	"bytes"
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

// The rehearsal gate parses the user's own code before any tofu runs and
// refuses what it does not name (ADR-0042 § The gate bar). It shares only
// leaf helpers with the Layer 3 gate, never its drivers: Layer 3 gates code
// infrafactory generated, this gate gates code the user wrote.
//
// Two rules have their own file: provider "aws" arguments and goal 11
// (rehearse_gate_aws.go), and paths (rehearse_gate_paths.go). Module
// sources, and the walk from the root through every module dir they
// reach, have their own file too (rehearse_gate_modules.go).

var errRehearsalRefuses = errors.New("the rehearsal gate refuses this configuration")

// rehearsalGateInput is what one gate run reads: the root module dir,
// module dirs to gate beside those the root's module blocks reach (gate 2
// adds the manifest's to them), the copy they live in (problems name
// files relative to it), the --region, the --var-file copy ("" when
// none was given), the hashicorp/aws version the lock pins, and ours:
// infrafactory's own override files, by the exact name it writes, each
// with the exact bytes it writes. A re-gate passes ours; prepare does not,
// since it writes them after its gates.
type rehearsalGateInput struct {
	root       string
	modules    []rehearsalModuleDir
	boundary   string
	region     string
	varFile    string
	awsVersion string
	ours       map[string][]byte
}

// rehearsalRefusedSuffixes are files tofu loads, or tofu test reads, that
// the gate does not parse. They are refused by name, never skipped.
var rehearsalRefusedSuffixes = []string{
	".tf.json", ".tofu", ".tofu.json", ".tfvars.json",
	".tftest.hcl", ".tftest.json", ".tofutest.hcl", ".tofutest.json", ".tfmock.hcl",
}

var rehearsalTopLevelBlocks = map[string]bool{
	"terraform": true, "provider": true, "variable": true, "locals": true, "output": true,
	"resource": true, "data": true, "module": true, "moved": true, "import": true, "removed": true,
}

// rehearsalProviders are the provider types the gate admits, each only
// from the hashicorp namespace.
var rehearsalProviders = map[string]bool{
	"aws": true, "random": true, "null": true, "tls": true,
	"time": true, "cloudinit": true, "local": true, "archive": true,
}

// rehearsalTypes are the resource and data types the gate admits besides
// aws_*. A provider's type left off its list, a newer one included, is
// refused.
var rehearsalTypes = map[string]map[string]bool{
	"resource": {
		"terraform_data": true,
		"random_id":      true, "random_integer": true, "random_password": true, "random_pet": true,
		"random_shuffle": true, "random_string": true, "random_uuid": true, "random_bytes": true,
		"null_resource":   true,
		"tls_private_key": true, "tls_self_signed_cert": true, "tls_cert_request": true, "tls_locally_signed_cert": true,
		"time_offset": true, "time_rotating": true, "time_sleep": true, "time_static": true,
		"cloudinit_config": true,
		"local_file":       true, "local_sensitive_file": true,
		"archive_file": true,
	},
	"data": {
		"null_data_source": true,
		"tls_public_key":   true,
		"cloudinit_config": true,
		"local_file":       true, "local_sensitive_file": true,
		"archive_file": true,
	},
}

// rehearsalRequiredProviderKeys are the keys a required_providers entry may set.
var rehearsalRequiredProviderKeys = map[string]bool{"source": true, "version": true, "configuration_aliases": true}

// rehearsalGate runs the gate over the root, every module dir and the var
// file, and returns every problem in one error, sorted. It passes with the
// root's provider "aws" aliases, sorted, "" naming the unaliased block, so
// prepare can write one override block per alias.
func rehearsalGate(in rehearsalGateInput) ([]string, error) {
	problems, aliases := in.problems()
	if err := rehearsalRefusal(problems); err != nil {
		return nil, err
	}
	return aliases, nil
}

// problems checks infrafactory's own overrides, walks the root and
// in.modules, then every dir their module blocks resolve to, each dir
// once, then the var file, then goal 11. It returns the problems and the
// root's provider "aws" aliases.
func (in rehearsalGateInput) problems() ([]string, []string) {
	problems := in.oursProblems()
	uses, regions := &rehearsalPathUses{}, newRehearsalRegions()
	start := append([]rehearsalModuleDir{{dir: in.root}}, in.modules...)
	walkProblems := rehearsalWalk(start, func(m rehearsalModuleDir) []rehearsalModuleDir {
		dirProblems, calls := in.dirProblems(m, uses, regions)
		problems = append(problems, m.named(dirProblems)...)
		return calls
	})
	problems = append(problems, walkProblems...)
	problems = append(problems, uses.problems()...)
	if in.varFile != "" {
		var varProblems []string
		regions.varFile, varProblems = in.tfvars(in.varFile)
		problems = append(problems, varProblems...)
	}
	return append(problems, regions.problems(in.region)...), slices.Compact(slices.Sorted(slices.Values(regions.aliases)))
}

// rehearsalRefusal is every problem in one error, sorted and each once,
// or nil for none.
func rehearsalRefusal(problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("%w: %s", errRehearsalRefuses, strings.Join(slices.Compact(problems), "; "))
}

// display names a path relative to the copy.
func (in rehearsalGateInput) display(path string) string {
	if rel, err := filepath.Rel(in.boundary, path); err == nil {
		return rel
	}
	return path
}

// rehearsalModule is one gated module dir and the gate run that reads it:
// path.module is dir, path.root is in.root, key is dir's module key, and
// uses collects the run's templates and written paths, and regions what
// goal 11 reads. providers maps each local name its
// required_providers declare to the source, lowercased.
type rehearsalModule struct {
	in        rehearsalGateInput
	dir       string
	key       string
	uses      *rehearsalPathUses
	regions   *rehearsalRegions
	root      bool
	providers map[string]string
}

// dirProblems applies every rule to one module dir, recording its
// templates and written paths in uses and what goal 11 reads in regions,
// and returns the dirs its module blocks resolve to.
func (in rehearsalGateInput) dirProblems(dir rehearsalModuleDir, uses *rehearsalPathUses, regions *rehearsalRegions) ([]string, []rehearsalModuleDir) {
	files, tfvars, whole, problems := in.dirFiles(dir.dir)
	calls, callProblems := rehearsalModuleCalls(files, dir, in.boundary)
	problems = append(problems, callProblems...)
	m := rehearsalModule{in: in, dir: dir.dir, key: dir.key, uses: uses, regions: regions, root: dir.dir == in.root, providers: map[string]string{}}
	if m.root {
		regions.tfvars, regions.rootWhole = tfvars, whole
	}
	for file, body := range files {
		problems = append(problems, m.requiredProviderProblems(file, body)...)
	}
	for file, body := range files {
		for name, attr := range body.Attributes {
			problems = append(problems, rehearsalAt(file, attr.SrcRange, name)+": refused: a .tf file holds only blocks")
		}
		for _, block := range body.Blocks {
			problems = append(problems, m.blockProblems(file, block)...)
		}
	}
	return problems, calls
}

// dirFiles applies the file policy to one module dir and returns its .tf
// and .tfvars files parsed, by display name, and whether it read the dir
// and every .tf file in it. A .tf file holds only blocks. Any other
// file, and any subdirectory but the test directory, is admitted unparsed:
// tofu's configuration loader skips them
// (TestAssumption_LoaderIgnoresUnknownSuffixes). Tofu reads some of those
// files by other routes, and ADR-0042 § The extension and file policy
// names the rule that owns each.
func (in rehearsalGateInput) dirFiles(dir string) (map[string]*hclsyntax.Body, map[string]*hclsyntax.Body, bool, []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, false, []string{fmt.Sprintf("%s: cannot read the module directory: %v", in.display(dir), err)}
	}
	var problems []string
	whole := true
	files, tfvars := map[string]*hclsyntax.Body{}, map[string]*hclsyntax.Body{}
	for _, entry := range entries {
		if _, ours := in.ours[entry.Name()]; ours && dir == in.root {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		lower := strings.ToLower(entry.Name())
		if problem := rehearsalEntryProblem(lower); problem != "" {
			problems = append(problems, in.display(path)+": "+problem)
			continue
		}
		switch {
		case strings.HasSuffix(lower, ".tfvars"):
			body, varProblems := in.tfvars(path)
			problems = append(problems, varProblems...)
			if body != nil {
				tfvars[in.display(path)] = body
			}
		case strings.HasSuffix(lower, ".tf"):
			body, problem := rehearsalParse(path, in.display(path))
			if problem != "" {
				problems, whole = append(problems, problem), false
				continue
			}
			files[in.display(path)] = body
		}
	}
	return files, tfvars, whole, problems
}

// oursProblems admits each of infrafactory's own override files in the
// root only when its bytes are exactly ours, and never parses it; dirFiles
// skips them in the root. A
// missing one is refused: an override that is not there is a wall that is
// down. Another spelling of its name, or the file in a module dir, is a
// user override, which rehearsalEntryProblem refuses: a case-sensitive
// filesystem holds it beside ours, and tofu loads it as an ordinary file.
func (in rehearsalGateInput) oursProblems() []string {
	var problems []string
	for name, want := range in.ours {
		path := filepath.Join(in.root, name)
		got, problem := rehearsalRead(path, in.display(path))
		switch {
		case problem != "":
			problems = append(problems, problem+": infrafactory's own override must be there")
		case !bytes.Equal(got, want):
			problems = append(problems, in.display(path)+": refused: it is not byte-identical to the override infrafactory writes")
		}
	}
	return problems
}

// rehearsalEntryProblem is why a dir entry, by its lowercased name, is
// refused, or "". Names compare lowercased because a case-insensitive
// filesystem maps ZZ_OVERRIDE.TF onto the name it spells.
func rehearsalEntryProblem(lower string) string {
	if lower == "terraform.d" {
		return "a terraform.d entry is refused: tofu reads it as a local provider mirror"
	}
	if lower == "tests" {
		return "a tests entry is refused: tofu reads it as the test directory (TestAssumption_LoaderIgnoresUnknownSuffixes)"
	}
	for _, suffix := range rehearsalRefusedSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return fmt.Sprintf("%s files are refused by name: tofu reads them and the gate does not parse them", suffix)
		}
	}
	for _, pattern := range []string{"override.tf*", "*_override.tf*"} {
		if matched, _ := filepath.Match(pattern, lower); matched {
			return "an override file is refused: it merges after infrafactory's overrides and can undo them (TestAssumption_UserOverrideMergesAfterOurs)"
		}
	}
	return ""
}

// errRehearsalNotRegular is a file rehearsalReadRegular refuses unread.
var errRehearsalNotRegular = errors.New("not a regular file")

// rehearsalReadRegular reads path only when Lstat finds a regular file, so
// a symlink or a FIFO is errRehearsalNotRegular, never followed or blocked
// on. An Lstat error is returned as it is, fs.ErrNotExist included.
func rehearsalReadRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	switch {
	case err != nil:
		return nil, err
	case !info.Mode().IsRegular():
		return nil, errRehearsalNotRegular
	}
	return os.ReadFile(path)
}

// rehearsalRead reads a file the gate must vouch for. One that is not a
// regular file or cannot be read is refused: the gate cannot vouch for
// what it cannot read.
func rehearsalRead(path, name string) ([]byte, string) {
	src, err := rehearsalReadRegular(path)
	switch {
	case errors.Is(err, errRehearsalNotRegular):
		return nil, name + ": not a regular file, so the gate cannot read it"
	case err != nil:
		return nil, fmt.Sprintf("%s: cannot read it: %v", name, err)
	}
	return src, ""
}

// rehearsalParse parses a file the gate must read. One rehearsalRead
// refuses, or that does not parse, is refused.
func rehearsalParse(path, name string) (*hclsyntax.Body, string) {
	src, problem := rehearsalRead(path, name)
	if problem != "" {
		return nil, problem
	}
	file, diags := hclsyntax.ParseConfig(src, name, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Sprintf("%s: does not parse, so the gate cannot vouch for it: %s", name, diags.Error())
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, name + ": unexpected body type"
	}
	return body, ""
}

// tfvars parses a .tfvars file, nil when it does not parse, and refuses
// one holding anything but attributes with literal values. A reference or
// a function call does not evaluate without a context, so it is refused
// here.
func (in rehearsalGateInput) tfvars(path string) (*hclsyntax.Body, []string) {
	name := in.display(path)
	body, problem := rehearsalParse(path, name)
	if problem != "" {
		return nil, []string{problem}
	}
	var problems []string
	for _, block := range body.Blocks {
		problems = append(problems, rehearsalAt(name, block.DefRange(), rehearsalAddress(block))+
			": a .tfvars file holds only attributes, so a block is refused")
	}
	for _, attr := range body.Attributes {
		if _, diags := attr.Expr.Value(nil); diags.HasErrors() {
			problems = append(problems, rehearsalAt(name, attr.SrcRange, attr.Name)+
				": a .tfvars value must be a literal, so a reference or function call is refused")
		}
	}
	return body, problems
}

// rehearsalAt is the "file:line: address" every problem starts with.
func rehearsalAt(file string, rng hcl.Range, addr string) string {
	return fmt.Sprintf("%s:%d: %s", file, rng.Start.Line, addr)
}

// rehearsalAddress is a block's address: aws_instance.web, data.http.x,
// provider.aws, terraform.
func rehearsalAddress(block *hclsyntax.Block) string {
	if block.Type == "resource" {
		return strings.Join(block.Labels, ".")
	}
	return strings.Join(append([]string{block.Type}, block.Labels...), ".")
}

// rehearsalSource is the provider a source names, when it is admitted:
// hashicorp/<one of rehearsalProviders>. A host-qualified source is not.
func rehearsalSource(source string) (string, bool) {
	namespace, typ, _ := strings.Cut(source, "/")
	return typ, namespace == "hashicorp" && rehearsalProviders[typ]
}

// source is the provider source a local name resolves to: its
// required_providers entry, or hashicorp/<name> as tofu implies.
func (m rehearsalModule) source(local string) string {
	if source, ok := m.providers[local]; ok {
		return source
	}
	return "hashicorp/" + local
}

// requiredProviderProblems records every required_providers entry and
// refuses one whose source is not admitted, or whose local name differs
// from its source's type: `x = { source = "hashicorp/aws" }` would put an
// aws provider block out of reach of the aws rules.
func (m rehearsalModule) requiredProviderProblems(file string, body *hclsyntax.Body) []string {
	var problems []string
	for _, tf := range body.Blocks {
		if tf.Type != "terraform" {
			continue
		}
		for _, rp := range tf.Body.Blocks {
			if rp.Type != "required_providers" {
				continue
			}
			for _, inner := range rp.Body.Blocks {
				problems = append(problems, rehearsalAt(file, inner.DefRange(), "terraform.required_providers."+inner.Type)+": refused: required_providers holds only entries")
			}
			for local, attr := range rp.Body.Attributes {
				at := rehearsalAt(file, attr.SrcRange, "terraform.required_providers."+local)
				source, ok := rehearsalRequiredProvider(attr)
				if !ok {
					problems = append(problems, at+": not a literal entry the gate can read")
					continue
				}
				m.providers[local] = source
				typ, admitted := rehearsalSource(source)
				switch {
				case !admitted:
					problems = append(problems, fmt.Sprintf("%s: source %q is not admitted; the gate admits hashicorp/{%s}", at, source,
						strings.Join(slices.Sorted(maps.Keys(rehearsalProviders)), ", ")))
				case typ != local:
					problems = append(problems, fmt.Sprintf("%s: local name %q differs from its source's type %q, so its provider blocks would dodge that provider's rules", at, local, typ))
				}
			}
		}
	}
	return problems
}

// rehearsalRequiredProvider reads a required_providers entry's source,
// lowercased: its literal `source`, or hashicorp/<local name> when it
// names none (a bare version string included), as tofu implies. ok is
// false for anything else.
func rehearsalRequiredProvider(attr *hclsyntax.Attribute) (string, bool) {
	implied := "hashicorp/" + attr.Name
	switch expr := attr.Expr.(type) {
	case *hclsyntax.TemplateExpr:
		return implied, expr.IsStringLiteral()
	case *hclsyntax.ObjectConsExpr:
		source := implied
		for _, item := range expr.Items {
			key := hcl.ExprAsKeyword(item.KeyExpr)
			if !rehearsalRequiredProviderKeys[key] {
				return "", false
			}
			if key != "source" {
				continue
			}
			val, diags := item.ValueExpr.Value(nil)
			if diags.HasErrors() || val.IsNull() || !val.IsKnown() || val.Type() != cty.String {
				return "", false
			}
			source = strings.ToLower(val.AsString())
		}
		return source, true
	}
	return "", false
}

// blockProblems applies every rule to one top-level block.
func (m rehearsalModule) blockProblems(file string, block *hclsyntax.Block) []string {
	addr := rehearsalAddress(block)
	at := rehearsalAt(file, block.DefRange(), addr)
	var problems []string
	if !rehearsalTopLevelBlocks[block.Type] {
		problems = append(problems, fmt.Sprintf("%s: %q blocks are refused", at, block.Type))
	}
	switch block.Type {
	case "terraform":
		problems = append(problems, m.terraformProblems(file, block)...)
	case "provider":
		problems = append(problems, m.providerProblems(file, at, block)...)
	case "resource", "data":
		problems = append(problems, m.typeProblems(file, at, block)...)
	case "import":
		problems = append(problems, m.providerRefProblems(file, addr, block)...)
	case "module":
		problems = append(problems, m.moduleProvidersProblems(file, addr, block)...)
	case "variable":
		m.recordDefault(block)
	}
	return append(problems, m.bodyProblems(file, addr, block, block.Type == "variable")...)
}

// terraformProblems admits required_providers and required_version, and
// backend in the root only. encryption, cloud and anything else are
// refused by name.
func (m rehearsalModule) terraformProblems(file string, block *hclsyntax.Block) []string {
	var problems []string
	const admits = "terraform {} admits only required_providers, required_version and, in the root, backend"
	for name, attr := range block.Body.Attributes {
		if name != "required_version" {
			problems = append(problems, rehearsalAt(file, attr.SrcRange, "terraform."+name)+": refused: "+admits)
		}
	}
	for _, inner := range block.Body.Blocks {
		if inner.Type == "required_providers" || (inner.Type == "backend" && m.root) {
			continue
		}
		problems = append(problems, rehearsalAt(file, inner.DefRange(), "terraform."+rehearsalAddress(inner))+": refused: "+admits)
	}
	return problems
}

// providerProblems admits a provider block only for an admitted source.
// An aws one is refused in a child module, where infrafactory's root
// override does not reach it (TestAssumption_RootOverrideMissesChildModuleProvider),
// and held to the aws rules in the root. Any other admits only `alias`.
func (m rehearsalModule) providerProblems(file, at string, block *hclsyntax.Block) []string {
	if len(block.Labels) != 1 {
		return []string{at + ": a provider block takes one label"}
	}
	source := m.source(block.Labels[0])
	typ, admitted := rehearsalSource(source)
	switch {
	case !admitted:
		return []string{fmt.Sprintf("%s: provider source %q is not admitted", at, source)}
	case typ == "aws" && !m.root:
		return []string{at + `: a provider "aws" block in a child module is refused: infrafactory's root override does not reach it`}
	case typ == "aws":
		return m.awsProviderProblems(file, at, block)
	}
	var problems []string
	for name, attr := range block.Body.Attributes {
		if name != "alias" {
			problems = append(problems, fmt.Sprintf("%s: refused: a %s provider block admits only alias", rehearsalAt(file, attr.SrcRange, rehearsalAddress(block)+"."+name), typ))
		}
	}
	for _, inner := range block.Body.Blocks {
		problems = append(problems, fmt.Sprintf("%s: refused: a %s provider block admits only alias", rehearsalAt(file, inner.DefRange(), rehearsalAddress(block)+"."+inner.Type), typ))
	}
	return problems
}

// typeProblems admits aws_* types and the types rehearsalTypes names.
func (m rehearsalModule) typeProblems(file, at string, block *hclsyntax.Block) []string {
	if len(block.Labels) != 2 {
		return []string{fmt.Sprintf("%s: a %s block takes two labels", at, block.Type)}
	}
	typ := block.Labels[0]
	if strings.HasPrefix(typ, "aws_") {
		m.recordAWSRegion(file, block)
	} else if !rehearsalTypes[block.Type][typ] {
		return []string{fmt.Sprintf("%s: %s type %q is not admitted", at, block.Type, typ)}
	}
	problems := m.providerRefProblems(file, rehearsalAddress(block), block)
	return append(problems, m.pathArgProblems(file, block)...)
}

// providerRefProblems refuses a `provider = <name>[.<alias>]` that names
// a provider outside the admitted ones, which tofu would install.
func (m rehearsalModule) providerRefProblems(file, addr string, block *hclsyntax.Block) []string {
	attr, ok := block.Body.Attributes["provider"]
	if !ok {
		return nil
	}
	return m.providerRefProblem(rehearsalAt(file, attr.SrcRange, addr+".provider"), attr.Expr)
}

// providerRefProblem refuses one provider reference, <name>[.<alias>],
// that names a provider outside the admitted ones.
func (m rehearsalModule) providerRefProblem(at string, expr hcl.Expression) []string {
	traversal, diags := hcl.AbsTraversalForExpr(expr)
	if diags.HasErrors() {
		return []string{at + ": not a provider reference the gate can read"}
	}
	source := m.source(traversal.RootName())
	if _, admitted := rehearsalSource(source); !admitted {
		return []string{fmt.Sprintf("%s: provider source %q is not admitted", at, source)}
	}
	return nil
}

// bodyProblems walks a block at every depth: provisioner and
// connection are refused wherever they are, and every attribute's calls
// are checked. A top-level variable's `type` is skipped: list(string)
// parses as a call and names a type.
func (m rehearsalModule) bodyProblems(file, addr string, block *hclsyntax.Block, skipType bool) []string {
	var problems []string
	for name, attr := range block.Body.Attributes {
		if skipType && name == "type" {
			continue
		}
		problems = append(problems, m.callProblems(rehearsalAt(file, attr.SrcRange, addr+"."+name), attr.Expr)...)
	}
	for _, inner := range block.Body.Blocks {
		innerAddr := addr + "." + rehearsalAddress(inner)
		if inner.Type == "provisioner" || inner.Type == "connection" {
			problems = append(problems, fmt.Sprintf("%s: %q blocks are refused at any depth: they run commands on this machine or another", rehearsalAt(file, inner.DefRange(), innerAddr), inner.Type))
		}
		problems = append(problems, m.bodyProblems(file, innerAddr, inner, false)...)
	}
	return problems
}

// callProblems holds every call in expr to the function rule and the path
// functions' arguments to the path rule.
func (m rehearsalModule) callProblems(at string, expr hclsyntax.Expression) []string {
	return append(rehearsalFunctionProblems(at, expr), m.pathCallProblems(at, expr)...)
}

// rehearsalFunctionProblems admits a call only to a function on the Layer 3
// pure-function allowlist or a path function. A namespaced name
// (core::file, provider::local::direxists) is refused by name.
func rehearsalFunctionProblems(at string, expr hclsyntax.Expression) []string {
	var problems []string
	for _, fn := range layer3CallsIn(expr) {
		switch {
		case strings.Contains(fn, "::"):
			problems = append(problems, fmt.Sprintf("%s: calls %s(): a namespaced function is refused by name", at, fn))
		case layer3PureFunctions[fn], rehearsalPathFunctions[fn]:
		default:
			problems = append(problems, fmt.Sprintf("%s: calls %s(), which is not on the pure-function allowlist", at, fn))
		}
	}
	return problems
}
