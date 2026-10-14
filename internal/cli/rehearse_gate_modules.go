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
// init does with it: only a literal ./ or ../ source, with no backslash
// and crossing no symlink, that resolves to a directory inside the copy
// is admitted, and every download is refused, registry modules included.
// Gate 1 holds each module block to it and gates every dir the blocks
// reach, refusing a cycle, before any tofu; gate 2 holds the manifest the
// first init writes to it.

const rehearsalModulesAdmit = "only a literal ./ or ../ source inside the boundary is admitted: every module download is refused"

// rehearsalModuleDir is a module dir and its key: the module path tofu's
// manifest names it by (m.s), "" for the root. call names the module
// block's source that reached it, "" for a dir the walk starts from.
type rehearsalModuleDir struct{ key, dir, call string }

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

// rehearsalWalk visits each start dir, then, depth first, every dir a
// visited dir's module blocks resolve to, each dir once; visit returns
// those calls. A call whose target is already on its own call path, the
// start dir included, is a cycle: it is refused naming the call, and not
// followed. Dirs compare resolved, so two spellings of one dir are one.
func rehearsalWalk(start []rehearsalModuleDir, visit func(rehearsalModuleDir) []rehearsalModuleDir) []string {
	visited := map[string]bool{}
	var problems []string
	var walk func(m rehearsalModuleDir, path map[string]bool)
	walk = func(m rehearsalModuleDir, path map[string]bool) {
		dir := rehearsalWalkKey(m.dir)
		visited[dir], path[dir] = true, true
		defer delete(path, dir)
		for _, call := range visit(m) {
			switch target := rehearsalWalkKey(call.dir); {
			case path[target]:
				problems = append(problems, m.named([]string{call.call + " is refused: it resolves to a module dir already on its call path, a cycle"})...)
			case !visited[target]:
				walk(call, path)
			}
		}
	}
	for _, m := range start {
		if !visited[rehearsalWalkKey(m.dir)] {
			walk(m, map[string]bool{})
		}
	}
	return problems
}

// rehearsalWalkKey is a dir resolved, or cleaned when it does not resolve:
// reading it then fails, and that is reported where it is read.
func rehearsalWalkKey(dir string) string {
	if resolved, err := rehearsalResolve(dir); err == nil {
		return resolved
	}
	return filepath.Clean(dir)
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
			target, call, blockProblems := rehearsalModuleSource(file, parent.dir, boundary, block)
			problems = append(problems, blockProblems...)
			if target == "" {
				continue
			}
			key := block.Labels[0]
			if parent.key != "" {
				key = parent.key + "." + key
			}
			calls = append(calls, rehearsalModuleDir{key: key, dir: target, call: call})
		}
	}
	return calls, problems
}

// rehearsalModuleSource holds one module block in dir to the rule, and
// returns the dir its source resolves to under boundary, the call
// (<file>:<line>: module.<name>.source: source "./m"), and the
// problems.
func rehearsalModuleSource(file, dir, boundary string, block *hclsyntax.Block) (string, string, []string) {
	addr := rehearsalAddress(block)
	var problems []string
	if attr, ok := block.Body.Attributes["version"]; ok {
		problems = append(problems, rehearsalAt(file, attr.SrcRange, addr+".version")+": refused: a local module takes no version")
	}
	attr, ok := block.Body.Attributes["source"]
	if !ok {
		return "", "", append(problems, rehearsalAt(file, block.DefRange(), addr)+": refused: no source; "+rehearsalModulesAdmit)
	}
	at := rehearsalAt(file, attr.SrcRange, addr+".source")
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() || val.IsNull() || !val.IsKnown() || val.Type() != cty.String {
		return "", "", append(problems, fmt.Sprintf("%s: refused: the source is not a literal string (it reads %s); %s", at, rehearsalReads(attr.Expr), rehearsalModulesAdmit))
	}
	source := val.AsString()
	call := fmt.Sprintf("%s: source %q", at, source)
	switch {
	case !rehearsalLocalSource(source):
		return "", "", append(problems, call+" is refused: "+rehearsalModulesAdmit)
	case strings.Contains(source, `\`):
		return "", "", append(problems, call+` is refused: tofu reads a \ as a path separator, so a source holding one is refused by name`)
	}
	target, problem := rehearsalInside(filepath.Join(dir, source), boundary)
	if problem == "" {
		problem = rehearsalSourceLinkProblem(dir, source, boundary)
	}
	if problem != "" {
		return "", "", append(problems, call+" "+problem)
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		return "", "", append(problems, call+" is refused: it is not a directory")
	}
	return target, call, problems
}

// rehearsalSourceLinkProblem refuses a module source that crosses a
// symlink at any component. Tofu joins the source to the calling dir as
// written and cleans it lexically, so path.module would spell the link
// while the gate checks the dir it resolves to, and a
// "${path.module}/../.." would reach somewhere else. dir resolves, and a
// resolved dir's ancestors are real dirs, so a leading .. crosses none.
func rehearsalSourceLinkProblem(dir, source, boundary string) string {
	path, err := rehearsalResolve(dir)
	if err != nil {
		return fmt.Sprintf("is refused: cannot check it for a symlink: %v", err)
	}
	rel, err := filepath.Rel(path, filepath.Join(path, source))
	if err != nil {
		return fmt.Sprintf("is refused: cannot check it for a symlink: %v", err)
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		path = filepath.Join(path, part)
		if part == ".." {
			continue
		}
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Sprintf("is refused: cannot check it for a symlink: %v", err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Sprintf("is refused: it crosses the symlink %s, and tofu joins the source as written, so path.module there would not be the dir the gate checks", rehearsalBoundaryName(path, boundary))
		}
	}
	return ""
}

// rehearsalBoundaryName names a resolved-parent path relative to the
// boundary, resolved too, or whole when it is not under it.
func rehearsalBoundaryName(path, boundary string) string {
	if root, err := rehearsalResolve(boundary); err == nil && rehearsalUnder(path, root) {
		rel, _ := filepath.Rel(root, path)
		return filepath.ToSlash(rel)
	}
	return path
}

// moduleProvidersProblems holds both sides of every pair in a module
// block's providers map to the provider reference rule: a name with no
// required_providers entry implies hashicorp/<name>, which init would
// install.
func (m rehearsalModule) moduleProvidersProblems(file, addr string, block *hclsyntax.Block) []string {
	attr, ok := block.Body.Attributes["providers"]
	if !ok {
		return nil
	}
	at := rehearsalAt(file, attr.SrcRange, addr+".providers")
	providers, ok := attr.Expr.(*hclsyntax.ObjectConsExpr)
	if !ok {
		return []string{at + ": not a providers map the gate can read"}
	}
	var problems []string
	for _, item := range providers.Items {
		problems = append(problems, m.providerRefProblem(at, item.KeyExpr)...)
		problems = append(problems, m.providerRefProblem(at, item.ValueExpr)...)
	}
	return problems
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
// reach, transitively, each resolved inside boundary and listed once, and
// never dir itself: a call back onto its own call path is a cycle. It
// reads each dir by the gate's walk, so anything that stops the gate
// reading one (a module source the rule refuses, a .tf file that does not
// parse, a file refused by name) refuses the lot. Prepare uses it to find
// what to copy.
func rehearsalLocalModuleSources(dir, boundary string) ([]string, error) {
	dirs, problems := rehearsalGateInput{root: dir, boundary: boundary}.moduleDirs()
	if err := rehearsalRefusal(problems); err != nil {
		return nil, err
	}
	return dirs, nil
}

// moduleDirs walks the root's module blocks as the gate does, without
// the block rules, and returns every module dir it visits, never the root,
// and the problems found reading them.
func (in rehearsalGateInput) moduleDirs() ([]string, []string) {
	var dirs, problems []string
	walkProblems := rehearsalWalk([]rehearsalModuleDir{{dir: in.root}}, func(m rehearsalModuleDir) []rehearsalModuleDir {
		if m.call != "" {
			dirs = append(dirs, m.dir)
		}
		files, _, _, fileProblems := in.dirFiles(m.dir)
		calls, callProblems := rehearsalModuleCalls(files, m, in.boundary)
		problems = append(problems, m.named(append(fileProblems, callProblems...))...)
		return calls
	})
	return dirs, append(problems, walkProblems...)
}

// rehearsalManifestEntry is one entry of the manifest the first init
// writes (TestAssumption_ModulesManifestShape).
type rehearsalManifestEntry struct{ Key, Source, Dir string }

// rehearsalModulesGate is gate 2: after the first init, it holds every
// entry of the manifest under data to the rule, then runs the full gate
// over the root, the caller's module dirs, every admitted module dir and
// every dir their module blocks reach, naming each module's key, and the
// var file.
func rehearsalModulesGate(in rehearsalGateInput, data string) error {
	modules, problems := in.manifestModules(data)
	in.modules = slices.Concat(in.modules, modules)
	gateProblems, _ := in.problems()
	return rehearsalRefusal(append(problems, gateProblems...))
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
		dirs, problems := in.moduleDirs()
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
