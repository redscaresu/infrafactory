package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// The module source rule (ADR-0042 Decision 2) judges a source by what
// init does with it: only a literal ./ or ../ source that resolves to a
// directory inside the copy is admitted, and every download is refused,
// registry modules included. Gate 1 holds each module block to it and
// gates every dir the blocks reach before any tofu; gate 2 holds the
// manifest the first init writes to it.

const rehearsalModulesAdmit = "only a literal ./ or ../ source inside the boundary is admitted: every module download is refused"

// rehearsalModuleDir is a module dir and its key: the module path tofu's
// manifest names it by (m.s), "" for the root.
type rehearsalModuleDir struct{ key, dir string }

// named prefixes problems found in the module dir with its key.
func (m rehearsalModuleDir) named(problems []string) []string {
	if m.key == "" {
		return problems
	}
	out := make([]string, len(problems))
	for i, problem := range problems {
		out[i] = fmt.Sprintf("module %q: %s", m.key, problem)
	}
	return out
}

// rehearsalWalk visits each start dir, then every dir a visited dir's
// module blocks resolve to, each dir once. visit returns those calls.
func rehearsalWalk(start []rehearsalModuleDir, visit func(rehearsalModuleDir) []rehearsalModuleDir) {
	visited := map[string]bool{}
	for queue := start; len(queue) > 0; queue = queue[1:] {
		if dir := filepath.Clean(queue[0].dir); !visited[dir] {
			visited[dir] = true
			queue = append(queue, visit(queue[0])...)
		}
	}
}

// rehearsalModuleCalls holds every module block in one module dir's files
// to the rule, and returns the dirs they resolve to, keyed.
func rehearsalModuleCalls(files map[string]*hclsyntax.Body, parent rehearsalModuleDir, boundary string) ([]rehearsalModuleDir, []string) {
	var calls []rehearsalModuleDir
	var problems []string
	for _, file := range slices.Sorted(maps.Keys(files)) {
		for _, block := range files[file].Blocks {
			if block.Type != "module" {
				continue
			}
			if len(block.Labels) != 1 {
				problems = append(problems, rehearsalAt(file, block.DefRange(), rehearsalAddress(block))+": a module block takes one label")
				continue
			}
			target, blockProblems := rehearsalModuleSource(file, parent.dir, boundary, block)
			problems = append(problems, blockProblems...)
			if target == "" {
				continue
			}
			key := block.Labels[0]
			if parent.key != "" {
				key = parent.key + "." + key
			}
			calls = append(calls, rehearsalModuleDir{key: key, dir: target})
		}
	}
	return calls, problems
}

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
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		return "", append(problems, fmt.Sprintf("%s: source %q is refused: it is not a directory", at, source))
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
	if !rehearsalUnder(resolved, root) {
		return "", fmt.Sprintf("resolves to %s, outside the boundary %s", resolved, root)
	}
	rel, _ := filepath.Rel(root, resolved)
	return filepath.Join(boundary, rel), ""
}

func rehearsalResolve(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// rehearsalUnder reports whether path lies in dir or under it, lexically.
func rehearsalUnder(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && filepath.IsLocal(rel)
}

// rehearsalLocalModuleSources is every module dir that dir's module blocks
// reach, transitively, each resolved inside boundary and listed once. It
// reads each dir by the gate's walk, so anything that stops the gate
// reading one (a module source the rule refuses, a .tf file that does not
// parse, a file refused by name) refuses the lot. rehearsal-prepare-copy
// uses it to find what to copy.
func rehearsalLocalModuleSources(dir, boundary string) ([]string, error) {
	dirs, problems := rehearsalModuleDirs(dir, boundary)
	if err := rehearsalRefusal(problems); err != nil {
		return nil, err
	}
	return dirs, nil
}

// rehearsalModuleDirs walks root's module blocks as the gate does, without
// the block rules, and returns every dir a block resolves to, once, and
// the problems found reading them.
func rehearsalModuleDirs(root, boundary string) ([]string, []string) {
	in := rehearsalGateInput{root: root, boundary: boundary}
	var dirs, problems []string
	seen := map[string]bool{}
	rehearsalWalk([]rehearsalModuleDir{{dir: root}}, func(m rehearsalModuleDir) []rehearsalModuleDir {
		files, fileProblems := in.dirFiles(m.dir)
		calls, callProblems := rehearsalModuleCalls(files, m, boundary)
		problems = append(problems, m.named(append(fileProblems, callProblems...))...)
		for _, call := range calls {
			if !seen[call.dir] {
				seen[call.dir] = true
				dirs = append(dirs, call.dir)
			}
		}
		return calls
	})
	return dirs, problems
}

// rehearsalManifestEntry is one entry of the manifest the first init
// writes (TestAssumption_ModulesManifestShape).
type rehearsalManifestEntry struct{ Key, Source, Dir string }

// rehearsalModulesGate is gate 2: after the first init, it holds every
// entry of the manifest under data to the rule, then runs the full gate
// over the root, every admitted module dir and every dir their module
// blocks reach, naming each module's key, and the var file.
func rehearsalModulesGate(in rehearsalGateInput, data string) error {
	modules, problems := in.manifestModules(data)
	in.modules = modules
	return rehearsalRefusal(append(problems, in.problems()...))
}

// manifestModules reads <data>/modules/modules.json and returns its
// module entries with each Dir resolved under the copy, and the problems.
// Tofu writes no manifest for a root with no module blocks, and only then
// is a missing one admitted. One root entry, Key "" Source "" Dir ".", is
// admitted; each other entry must have a local Source and a Dir that,
// relative to the root unless absolute, resolves inside the copy and not
// under <data>/modules, where the first init puts what it downloads.
func (in rehearsalGateInput) manifestModules(data string) ([]rehearsalModuleDir, []string) {
	const name = "modules.json"
	fetched := filepath.Join(data, "modules")
	raw, err := os.ReadFile(filepath.Join(fetched, name))
	if errors.Is(err, fs.ErrNotExist) {
		dirs, problems := rehearsalModuleDirs(in.root, in.boundary)
		if len(dirs) == 0 && len(problems) == 0 {
			return nil, nil
		}
		return nil, append(problems, name+": refused: the first init wrote no manifest, which only a root with no module blocks may lack")
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: cannot read it: %v", name, err)}
	}
	var manifest struct{ Modules []rehearsalManifestEntry }
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, []string{fmt.Sprintf("%s: does not parse, so the gate cannot vouch for it: %v", name, err)}
	}
	var modules []rehearsalModuleDir
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
			modules = append(modules, rehearsalModuleDir{key: entry.Key, dir: dir})
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
	if rehearsalFetched(dir, fetched) {
		return "", append(problems, fmt.Sprintf("dir %q lies under %s, where the first init puts what it downloads", entry.Dir, fetched))
	}
	inside, problem := rehearsalInside(dir, in.boundary)
	if problem != "" {
		return "", append(problems, fmt.Sprintf("dir %q %s", entry.Dir, problem))
	}
	return inside, problems
}

// rehearsalFetched reports whether dir lies under fetched as spelled, or
// once either is resolved. The spelling is checked first: a module fetched
// from a local path is a symlink there to where it was fetched from
// (TestAssumption_ModulesManifestShape), so its resolved path is not
// under fetched at all.
func rehearsalFetched(dir, fetched string) bool {
	resolvedFetched, _ := rehearsalResolve(fetched)
	resolvedDir, _ := rehearsalResolve(dir)
	return rehearsalUnder(dir, fetched) || rehearsalUnder(dir, resolvedFetched) || rehearsalUnder(resolvedDir, resolvedFetched)
}
