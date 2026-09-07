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

// allowedErrorText is every place this package may touch an error's text,
// each with the reason it is allowed.
//
// An entry is a deliberate, review-visible act. Adding one is how a new
// exception gets made; the alternative -- a rule in a comment -- is what
// produced sixty-five violations.
var allowedErrorText = map[string]string{
	"deployment_actions.go:Error":                   "implements the error interface; this IS the text",
	"server.go:writeRequestError":                   "the one deliberate echo, for the caller's own payload",
	"handlers_runs.go:handleIterationFiles":         "matches on the text to classify a sentinel; nothing is written",
	"handlers_runs.go:handleRunArtifact":            "as above",
	"handlers_runs.go:handleRunFiles":               "as above",
	"handlers_scenarios.go:validateScenarioHandler": "extractYAMLSyntaxDetail strips the leaky prefix and keeps the syntax detail",
	"handlers_scenarios.go:handlePutScenarioByPath": "same, for the save path: a reader editing YAML needs their own syntax error",
	"handlers_scenarios.go:scenarioByPathHandler":   "resolveScenarioFile's refusals are literals it composed; the one that wraps ours is sentinel-checked and withheld",
}

// An error's TEXT may not be handled outside the places named above.
//
// # Why the rule is about the text and not about a function
//
// The first version of this audit asked "was `writeJSONError` called with
// an error?" and its own comment claimed it covered every body-writer. It
// did not, and the claim was the giveaway: `writeJSON` is the bigger door,
// and error text reaches a response through it as a STRUCT FIELD --
// `payload.Unreadable = append(..., e.Error())` and `ParseError:
// err.Error()` both sailed past while the audit reported green. A local
// variable defeated it just as easily: `msg := "read: " + err.Error()`
// then `writeJSONError(w, status, msg)`.
//
// Asking about the text instead makes the door irrelevant. There is no
// spelling of "put this error in a response" that does not first call
// `.Error()` somewhere in the package.
//
// # What to do instead
//
//   - `state.writeInternalError(w, status, message, err)` -- logs the
//     cause and answers with a stable sentence.
//   - `writeRequestError` -- echoes, for errors describing the caller's
//     own payload.
//   - `state.logDetail` -- anywhere else the cause needs recording.
//
// # What this is and is not about
//
// It is NOT a remote information-disclosure defence. The server binds
// `127.0.0.1` and answers loopback origins only (ADR-0026), so the reader
// of these bodies is the operator, on their own machine, looking at their
// own paths.
//
// It is two other things. `open /Users/x/.infrafactory/live/dep-1.json:
// permission denied` in a red banner is noise the reader cannot act on,
// where "dep-1 could not be read; see the server log" names the record and
// puts the cause where it belongs. And loopback is a DEPLOYMENT default --
// `--addr` overrides it -- so a code-level habit of composing what we
// send is worth having independently of how it happens to be served
// today.
//
// # Not a status-code rule
//
// It would be convenient if 5xx meant hide and 4xx meant show.
// `handlePutScenarioByPath` answers 400 for malformed YAML from an error
// carrying the schema path, because the parse runs against a temp file.
// Where the error came from is what matters, not what the status says
// about blame.
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

		file, parseErr := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ParseComments)
		require.NoError(t, parseErr, name)
		audited++

		// Walked per top-level declaration so the enclosing function is
		// known: the allowlist is keyed by it, which keeps an exception
		// to the site that earned it rather than the whole file. Skipping
		// all of server.go to spare one helper left every handler
		// defined there unaudited.
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			key := name + ":" + fn.Name.Name
			if _, allowed := allowedErrorText[key]; allowed {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Error" || len(call.Args) != 0 {
					return true
				}
				// `logDetail(..., err)` is where a cause is SUPPOSED to
				// go, and passing `err` rather than `err.Error()` is the
				// idiom -- so an explicit `.Error()` inside one is still
				// worth flagging as an oddity rather than silently fine.
				offenders = append(offenders,
					"  "+fset.Position(call.Pos()).String()+"  in "+fn.Name.Name+
						"  ("+render(sel.X)+".Error())")
				return true
			})
		}
	}

	require.Greater(t, audited, 0, "the audit found no files to read, which is not a pass")

	assert.Empty(t, offenders, strings.Join(append([]string{
		"An error's text is being handled outside the places allowed to.",
		"",
		"Use state.writeInternalError(w, status, message, err): it logs the cause and",
		"answers with a stable sentence. If the error describes the CALLER's own payload,",
		"use writeRequestError. If it just needs recording, use state.logDetail.",
		"",
		"If this really is a legitimate exception, add it to allowedErrorText with the",
		"reason -- which is a thing a reviewer can see and argue with.",
		"",
	}, offenders...), "\n"))
}

func render(expr ast.Expr) string {
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		return render(sel.X) + "." + sel.Sel.Name
	}
	return "<expr>"
}
