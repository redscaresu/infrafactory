package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// The module source rule (ADR-0042 Decision 2) judges a source by what
// init does with it: only a literal ./ or ../ source that resolves inside
// the copy is admitted, and every download is refused, registry modules
// included. Gate 1 holds each module block to it before any tofu; gate 2
// holds the manifest the first init writes to it.

const rehearsalModulesAdmit = "only a literal ./ or ../ source inside the boundary is admitted: every module download is refused"

// rehearsalModuleSource holds one module block in dir to the rule, and
// returns the dir its source resolves to under boundary, and the problems.
func rehearsalModuleSource(file, dir, boundary string, block *hclsyntax.Block) (string, []string) {
	addr := rehearsalAddress(block)
	var problems []string
	if attr, ok := block.Body.Attributes["version"]; ok {
		problems = append(problems, rehearsalAt(file, attr.SrcRange, addr+".version")+": refused: a local module takes no version")
	}
	attr, ok := block.Body.Attributes["source"]
	if !ok {
		return "", append(problems, rehearsalAt(file, block.DefRange(), addr)+": refused: no source; "+rehearsalModulesAdmit)
	}
	at := rehearsalAt(file, attr.SrcRange, addr+".source")
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() || val.IsNull() || !val.IsKnown() || val.Type() != cty.String {
		return "", append(problems, fmt.Sprintf("%s: refused: the source is not a literal string (it reads %s); %s", at, rehearsalReads(attr.Expr), rehearsalModulesAdmit))
	}
	source := val.AsString()
	if !rehearsalLocalSource(source) {
		return "", append(problems, fmt.Sprintf("%s: source %q is refused: %s", at, source, rehearsalModulesAdmit))
	}
	target, problem := rehearsalInside(filepath.Join(dir, source), boundary)
	if problem != "" {
		return "", append(problems, fmt.Sprintf("%s: source %q %s", at, source, problem))
	}
	return target, problems
}

func rehearsalLocalSource(source string) bool {
	return strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../")
}

// rehearsalReads names the references a non-literal expression reads.
func rehearsalReads(expr hclsyntax.Expression) string {
	var refs []string
	for _, traversal := range expr.Variables() {
		refs = append(refs, string(hclwrite.TokensForTraversal(traversal).Bytes()))
	}
	if len(refs) == 0 {
		return "no reference, but does not evaluate as a literal"
	}
	return strings.Join(refs, ", ")
}

// rehearsalInside resolves path (Abs, then EvalSymlinks) and, when it lies
// inside boundary (resolved too), returns it as a path under boundary;
// otherwise it returns why not.
func rehearsalInside(path, boundary string) (string, string) {
	resolved, err := rehearsalResolve(path)
	if err != nil {
		return "", fmt.Sprintf("does not resolve: %v", err)
	}
	root, err := rehearsalResolve(boundary)
	if err != nil {
		return "", fmt.Sprintf("cannot be checked: the boundary does not resolve: %v", err)
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Sprintf("resolves to %s, outside the boundary %s", resolved, root)
	}
	return filepath.Join(boundary, rel), ""
}

func rehearsalResolve(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// rehearsalLocalModuleSources is the dirs dir's module blocks call, each
// resolved inside boundary. A block the rule refuses, or a .tf file that
// does not parse, refuses the lot. rehearsal-prepare-copy follows it to
// find what to copy.
func rehearsalLocalModuleSources(dir, boundary string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: cannot read the module directory: %v", errRehearsalRefuses, dir, err)
	}
	display := rehearsalGateInput{boundary: boundary}.display
	var sources, problems []string
	for _, entry := range entries {
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".tf") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		body, problem := rehearsalParse(path, display(path))
		if problem != "" {
			problems = append(problems, problem)
			continue
		}
		for _, block := range body.Blocks {
			if block.Type != "module" {
				continue
			}
			source, blockProblems := rehearsalModuleSource(display(path), dir, boundary, block)
			problems = append(problems, blockProblems...)
			sources = append(sources, source)
		}
	}
	if err := rehearsalRefusal(problems); err != nil {
		return nil, err
	}
	return sources, nil
}

// rehearsalManifestEntry is one entry of the manifest the first init
// writes (TestAssumption_ModulesManifestShape).
type rehearsalManifestEntry struct{ Key, Source, Dir string }

// rehearsalModulesGate is gate 2: after the first init, it holds every
// entry of the manifest under data to the rule, then runs the full gate
// over the root, the var file and every admitted module dir, naming each
// module's key.
func rehearsalModulesGate(in rehearsalGateInput, data string) error {
	modules, problems := in.manifestModules(data)
	in.modules = nil
	problems = append(problems, in.problems()...)
	for _, module := range modules {
		for _, problem := range in.dirProblems(module.Dir) {
			problems = append(problems, fmt.Sprintf("module %q: %s", module.Key, problem))
		}
	}
	return rehearsalRefusal(problems)
}

// manifestModules reads <data>/modules/modules.json and returns its
// module entries with each Dir resolved under the copy, and the problems.
// Tofu writes no manifest for a root with no module blocks, and only then
// is a missing one admitted. One root entry, Key "" Source "" Dir ".", is
// admitted; each other entry must have a local Source and a Dir that,
// relative to the root unless absolute, resolves inside the copy. A
// fetched module's Dir lies under <data>/modules, and a local module it
// calls keeps the Dir it was fetched from, so both are refused.
func (in rehearsalGateInput) manifestModules(data string) ([]rehearsalManifestEntry, []string) {
	const name = "modules.json"
	fetched := filepath.Join(data, "modules")
	raw, err := os.ReadFile(filepath.Join(fetched, name))
	if errors.Is(err, fs.ErrNotExist) {
		if sources, err := rehearsalLocalModuleSources(in.root, in.boundary); err == nil && len(sources) == 0 {
			return nil, nil
		}
		return nil, []string{name + ": refused: the first init wrote no manifest, which only a root with no module blocks may lack"}
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: cannot read it: %v", name, err)}
	}
	var manifest struct{ Modules []rehearsalManifestEntry }
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, []string{fmt.Sprintf("%s: does not parse, so the gate cannot vouch for it: %v", name, err)}
	}
	var modules []rehearsalManifestEntry
	var problems []string
	roots := 0
	for _, entry := range manifest.Modules {
		if entry.Key == "" {
			roots++
			if entry != (rehearsalManifestEntry{Dir: "."}) {
				problems = append(problems, fmt.Sprintf(`%s: refused: the root entry has Source %q and Dir %q, not "" and "."`, name, entry.Source, entry.Dir))
			}
			continue
		}
		dir, entryProblems := in.manifestEntryDir(entry, fetched)
		for _, problem := range entryProblems {
			problems = append(problems, fmt.Sprintf("%s: module %q: refused: %s", name, entry.Key, problem))
		}
		if len(entryProblems) == 0 {
			modules = append(modules, rehearsalManifestEntry{Key: entry.Key, Source: entry.Source, Dir: dir})
		}
	}
	if roots != 1 {
		problems = append(problems, fmt.Sprintf("%s: refused: it names %d root entries, not one", name, roots))
	}
	return modules, problems
}

// manifestEntryDir resolves a non-root entry's Dir under the copy, or
// says why it is refused.
func (in rehearsalGateInput) manifestEntryDir(entry rehearsalManifestEntry, fetched string) (string, []string) {
	var problems []string
	if !rehearsalLocalSource(entry.Source) {
		problems = append(problems, fmt.Sprintf("source %q: %s", entry.Source, rehearsalModulesAdmit))
	}
	dir := entry.Dir
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(in.root, dir)
	}
	if _, outside := rehearsalInside(dir, fetched); outside == "" {
		return "", append(problems, fmt.Sprintf("dir %q lies under %s, where the first init puts what it downloads", entry.Dir, fetched))
	}
	inside, problem := rehearsalInside(dir, in.boundary)
	if problem != "" {
		return "", append(problems, fmt.Sprintf("dir %q %s", entry.Dir, problem))
	}
	return inside, problems
}
