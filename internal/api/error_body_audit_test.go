package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No handler may put an error's TEXT into a response body.
//
// # Why this is a test and not a convention
//
// A Go error from the filesystem carries an absolute path.
// `*fs.PathError` renders as `open /Users/<name>/.infrafactory/runs/x:
// permission denied`, and every error body in this package is rendered
// verbatim by the UI -- `previewError`, `loadError`, `detailError` and
// the deploy outcome all print what the server sent.
//
// Writing that down as a rule did not work. A review round found four
// instances in `handlers_deployments.go`, each in code that had already
// been through a review of its own; fixing those four left SIXTY-ONE
// more of the same shape elsewhere in the package, in handlers nobody
// had thought to look at. The convention was known and the drift was
// invisible, which is what an audit is for -- the same reason this
// project has `cloud_prefix_lockstep_test.go` and
// `handlers/contract_audit_test.go`.
//
// # What is allowed instead
//
//   - `writeJSONError` — a message this package composed. Literals, and
//     values like a file's base name. Never an error.
//   - `writeInternalError` — logs the cause through the injected sink
//     and answers with a stable sentence. For anything a caller cannot
//     be shown.
//   - `writeRequestError` — the ONE deliberate echo, for errors that
//     describe the caller's own payload.
//
// # Not a status-code rule
//
// It would be convenient if 5xx meant "hide" and 4xx meant "show", and
// it is wrong: `handlePutScenarioByPath` answers **400** for malformed
// YAML from an error that carries the schema path, because the parse
// runs against a temp file. Safety is about where the error came from,
// not what the response says about blame.
func TestNoHandlerPutsAnErrorIntoAResponseBody(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var offenders []string
	audited := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// server.go DEFINES the two helpers that legitimately touch an
		// error's text. Auditing their bodies would forbid the very
		// seam this rule points callers at.
		if name == "server.go" {
			continue
		}

		file, parseErr := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ParseComments)
		require.NoError(t, parseErr, name)
		audited++

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			// EVERY body-writer, not just the obvious one.
			//
			// Auditing `writeJSONError` alone left `writeRefusal`
			// echoing an error two hundred lines from a fix for exactly
			// that -- safe by construction at the time, and one wrapped
			// *fs.PathError away from not being. A rule that covers one
			// door is a rule about that door.
			if !ok || !bodyWriters[fn.Name] {
				return true
			}
			for _, arg := range call.Args {
				if leaked := errorTextIn(arg); leaked != "" {
					offenders = append(offenders, formatOffence(fset, call.Pos(), name, fn.Name, leaked))
					break
				}
			}
			return true
		})
	}

	require.Greater(t, audited, 0, "the audit found no files to read, which is not a pass")

	assert.Empty(t, offenders, strings.Join(append([]string{
		"An error's text is being written into a response body.",
		"",
		"Use writeInternalError(w, state, status, message, err) instead: it logs the",
		"cause and answers with a stable sentence. If the error describes the CALLER's",
		"own payload -- a body that would not read, or JSON that would not decode --",
		"use writeRequestError, which echoes it deliberately.",
		"",
	}, offenders...), "\n"))
}

// bodyWriters are the functions that put a caller-supplied message into
// a response. `writeInternalError` and `writeRequestError` are absent
// deliberately: they TAKE an error, which is the point of them.
var bodyWriters = map[string]bool{
	"writeJSONError": true,
	"writeRefusal":   true,
}

// errorTextIn reports the offending expression, or "".
//
// It looks for two shapes: a call to `.Error()`, and a `%v`/`%s`/`%q`
// verb applied to something named like an error. Both were present in
// the package -- 50 of the first, 15 of the second -- and a rule that
// caught only the obvious one would have left a third of the class
// standing while reporting success.
func errorTextIn(expr ast.Expr) string {
	var found string
	ast.Inspect(expr, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		switch e := n.(type) {
		case *ast.CallExpr:
			if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Error" && len(e.Args) == 0 {
				found = render(sel.X) + ".Error()"
				return false
			}
			// fmt.Sprintf("...%v", err) and friends.
			if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Sprintf" {
				for _, a := range e.Args[1:] {
					if id, ok := a.(*ast.Ident); ok && looksLikeError(id.Name) {
						found = "fmt.Sprintf(..., " + id.Name + ")"
						return false
					}
				}
			}
		case *ast.Ident:
			// A bare error identifier concatenated in, e.g. "x: "+err.
			if looksLikeError(e.Name) {
				found = e.Name
				return false
			}
		}
		return true
	})
	return found
}

// looksLikeError matches the naming this package actually uses: `err`,
// `derr`, `validateErr`, `parseErr`. Deliberately a name check rather
// than a type check -- resolving types here would need the full
// type-checker for a rule whose whole value is being cheap enough that
// nobody is tempted to skip it.
func looksLikeError(name string) bool {
	lower := strings.ToLower(name)
	return lower == "err" || strings.HasSuffix(lower, "err")
}

func render(expr ast.Expr) string {
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return "<expr>"
}

func formatOffence(fset *token.FileSet, pos token.Pos, file, fnName, leaked string) string {
	p := fset.Position(pos)
	return "  " + file + ":" + itoa(p.Line) + "  " + fnName + "(... " + leaked + " ...)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
