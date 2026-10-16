package cli_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forbiddenStarters start a process without os/exec. None of them is used,
// so any reference to one fails.
var forbiddenStarters = map[string][]string{
	"os":                    {"StartProcess"},
	"syscall":               {"ForkExec", "Exec", "StartProcess"},
	"golang.org/x/sys/unix": {"Exec"},
}

// TestNoNewExecPath pins every file in the module that imports os/exec,
// with its count of exec.Command/exec.CommandContext references. Tofu must
// only run through execCommandRunner (exec_runner.go), whose Setpgid and
// single-SIGINT handling a new exec path would bypass, so adding one means
// editing this map on purpose.
func TestNoNewExecPath(t *testing.T) {
	want := map[string]int{
		"internal/api/formatter.go":            1,
		"internal/api/handlers_diagnostics.go": 0,
		"internal/cli/exec_runner.go":          1,
		"internal/cli/mock_start_starter.go":   4,
		"internal/cli/rehearse_fakeaws.go":     1,
		"internal/cli/ui_command.go":           0,
		"internal/e2e/helpers.go":              5,
		"internal/generator/claude_adapter.go": 1,
		"internal/runstore/commit.go":          2,
	}
	got, forbidden := scanExecPaths(t, repoRoot(t))
	assert.Empty(t, forbidden, "process starts outside os/exec")
	assert.Equal(t, want, got, "files importing os/exec, with their exec.Command/CommandContext count")
}

// scanExecPaths parses every non-test .go file the go tool would build in
// the module at root. It returns each file that imports os/exec with its
// exec.Command/CommandContext count, and every reference to a
// forbiddenStarters function or dot import of a watched package.
func scanExecPaths(t *testing.T, root string) (map[string]int, []string) {
	t.Helper()
	counts := map[string]int{}
	var forbidden []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skip, err := goToolIgnores(root, p, d); skip || err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		n, bad := scanFile(fset, f)
		if n >= 0 {
			counts[filepath.ToSlash(rel)] = n
		}
		forbidden = append(forbidden, bad...)
		return nil
	})
	require.NoError(t, err)
	return counts, forbidden
}

// goToolIgnores reports what the go tool leaves out of the module: names
// starting with "." or "_", testdata, and nested modules. It returns
// fs.SkipDir for an ignored directory.
func goToolIgnores(root, p string, d fs.DirEntry) (bool, error) {
	if p == root {
		return false, nil
	}
	name := d.Name()
	ignored := strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || (d.IsDir() && name == "testdata")
	if !ignored && d.IsDir() {
		_, err := os.Stat(filepath.Join(p, "go.mod"))
		ignored = err == nil
	}
	if ignored && d.IsDir() {
		return true, fs.SkipDir
	}
	return ignored, nil
}

// scanFile returns f's exec.Command/CommandContext reference count (-1 when
// f does not import os/exec) and each forbidden reference as "file:line: msg".
func scanFile(fset *token.FileSet, f *ast.File) (int, []string) {
	count := -1
	execName := ""
	watched := map[string]string{} // local name -> import path
	var bad []string
	for _, imp := range f.Imports {
		ip, _ := strconv.Unquote(imp.Path.Value)
		if ip != "os/exec" && forbiddenStarters[ip] == nil {
			continue
		}
		name := path.Base(ip)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." {
			bad = append(bad, fmt.Sprintf("%s: dot import of %s cannot be checked", fset.Position(imp.Pos()), ip))
			continue
		}
		if ip == "os/exec" {
			count, execName = 0, name
			continue
		}
		watched[name] = ip
	}
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if x.Name == execName && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext") {
			count++
		}
		for _, fn := range forbiddenStarters[watched[x.Name]] {
			if sel.Sel.Name == fn {
				bad = append(bad, fmt.Sprintf("%s: %s.%s starts a process outside os/exec", fset.Position(sel.Pos()), watched[x.Name], fn))
			}
		}
		return true
	})
	return count, bad
}
