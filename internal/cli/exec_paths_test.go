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

// execPackages build an exec.Cmd. execabs is a drop-in for os/exec.
var execPackages = map[string]bool{
	"os/exec":                  true,
	"golang.org/x/sys/execabs": true,
}

// forbiddenStarters start a process without os/exec. None of them is used,
// so any reference to one fails.
var forbiddenStarters = map[string][]string{
	"os":                    {"StartProcess"},
	"syscall":               {"ForkExec", "Exec", "StartProcess"},
	"golang.org/x/sys/unix": {"Exec"},
}

// TestNoNewExecPath pins every file in the module that imports an
// execPackages package, with its count of exec call sites: each
// Command/CommandContext reference, and each Cmd not behind a pointer (a
// literal, new(exec.Cmd), a var of type exec.Cmd), since a Cmd value can
// be filled in and started directly. No exec path is added without editing
// this map, so a new tofu runner cannot bypass execCommandRunner's
// (exec_runner.go) Setpgid and single-SIGINT handling unnoticed.
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
	assert.Equal(t, want, got, "files importing os/exec or execabs, with their exec call-site count")
}

// TestScanFileFixtures covers each scanFile rule, since today's tree uses
// only Command and CommandContext.
func TestScanFileFixtures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		src     string
		want    int
		wantBad []string
	}{
		{"Command", `package p
import "os/exec"
var _ = exec.Command("true")`, 1, nil},
		{"CommandContext", `package p
import "os/exec"
var _ = exec.CommandContext(nil, "true")`, 1, nil},
		{"Cmd literal", `package p
import "os/exec"
var _ = &exec.Cmd{Path: "true"}`, 1, nil},
		{"new Cmd", `package p
import "os/exec"
var _ = new(exec.Cmd)`, 1, nil},
		{"var Cmd", `package p
import "os/exec"
func f() error { var c exec.Cmd; return c.Start() }`, 1, nil},
		{"pointer Cmd parameter", `package p
import "os/exec"
func f(c *exec.Cmd) error { return c.Start() }`, 0, nil},
		{"LookPath only", `package p
import "os/exec"
var _, _ = exec.LookPath("tofu")`, 0, nil},
		{"renamed import", `package p
import osexec "os/exec"
var _ = osexec.Command`, 1, nil},
		{"execabs", `package p
import "golang.org/x/sys/execabs"
var _ = execabs.CommandContext`, 1, nil},
		{"dot import", `package p
import . "os/exec"
var _ = Command`, 0, []string{"fixture.go:2:8: dot import of os/exec cannot be checked"}},
		{"os.StartProcess", `package p
import "os"
var _ = os.StartProcess`, -1, []string{"fixture.go:3:9: os.StartProcess starts a process outside os/exec"}},
		{"syscall.Exec", `package p
import "syscall"
var _ = syscall.Exec`, -1, []string{"fixture.go:3:9: syscall.Exec starts a process outside os/exec"}},
		{"no os/exec", `package p
func f() {}`, -1, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "fixture.go", tc.src, parser.SkipObjectResolution)
			require.NoError(t, err)
			got, bad := scanFile(fset, f)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantBad, bad)
		})
	}
}

// scanExecPaths parses every non-test .go file in the module at root,
// whatever its build constraints. It returns each file that imports an
// execPackages package with its exec call-site count, and every reference
// to a forbiddenStarters function or dot import of a watched package.
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

// scanFile returns f's exec call-site count (-1 when f imports no
// execPackages package) and each forbidden reference as "file:line: msg".
func scanFile(fset *token.FileSet, f *ast.File) (int, []string) {
	names, importsExec, bad := watchedImports(fset, f)
	count := -1
	if importsExec {
		count = 0
	}
	pointee := map[ast.Expr]bool{} // X of each *X; a *exec.Cmd builds nothing
	ast.Inspect(f, func(n ast.Node) bool {
		if star, ok := n.(*ast.StarExpr); ok {
			pointee[star.X] = true
		}
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		name, ip := sel.Sel.Name, names[x.Name]
		if execPackages[ip] && (name == "Command" || name == "CommandContext" || (name == "Cmd" && !pointee[sel])) {
			count++
		}
		for _, fn := range forbiddenStarters[ip] {
			if name == fn {
				bad = append(bad, fmt.Sprintf("%s: %s.%s starts a process outside os/exec", fset.Position(sel.Pos()), ip, fn))
			}
		}
		return true
	})
	return count, bad
}

// watchedImports maps each local name f gives an execPackages or
// forbiddenStarters package to its import path, and reports whether f
// imports an execPackages package. A dot import cannot be checked, so it is
// returned in bad.
func watchedImports(fset *token.FileSet, f *ast.File) (map[string]string, bool, []string) {
	names := map[string]string{}
	importsExec := false
	var bad []string
	for _, imp := range f.Imports {
		ip, _ := strconv.Unquote(imp.Path.Value)
		if !execPackages[ip] && forbiddenStarters[ip] == nil {
			continue
		}
		importsExec = importsExec || execPackages[ip]
		name := path.Base(ip)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." {
			bad = append(bad, fmt.Sprintf("%s: dot import of %s cannot be checked", fset.Position(imp.Pos()), ip))
			continue
		}
		names[name] = ip
	}
	return names, importsExec, bad
}
