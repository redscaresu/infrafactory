package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// rehearsalPathFunctions read the path their first argument names
// (ADR-0042 § Paths and templates). Names match exactly, so a
// `::`-qualified spelling stays refused by the namespaced-function rule.
var rehearsalPathFunctions = map[string]bool{
	"file": true, "fileexists": true, "fileset": true, "filebase64": true,
	"filemd5": true, "filesha1": true, "filesha256": true, "filesha512": true,
	"filebase64sha256": true, "filebase64sha512": true, "templatefile": true,
}

// rehearsalPathArgs are the arguments that name a local path, by block
// kind, type and argument; true marks a path the apply writes. A dotted
// argument sits in nested blocks. The aws ones are every argument
// terraform-provider-aws v5.100.0 reads a local file from.
var rehearsalPathArgs = map[string]map[string]map[string]bool{
	"resource": {
		"local_file":                       {"filename": true, "source": false},
		"local_sensitive_file":             {"filename": true, "source": false},
		"archive_file":                     {"output_path": true, "source_file": false, "source_dir": false},
		"aws_s3_object":                    {"source": false},
		"aws_s3_bucket_object":             {"source": false},
		"aws_lambda_function":              {"filename": false},
		"aws_lambda_layer_version":         {"filename": false},
		"aws_synthetics_canary":            {"zip_file": false},
		"aws_gamelift_script":              {"zip_file": false},
		"aws_connect_contact_flow":         {"filename": false},
		"aws_connect_contact_flow_module":  {"filename": false},
		"aws_rds_custom_db_engine_version": {"filename": false},
		"aws_emr_cluster":                  {"configurations": false},
		"aws_dataexchange_revision_assets": {"asset.import_assets_from_signed_url.filename": false},
	},
	"data": {
		"local_file":           {"filename": false},
		"local_sensitive_file": {"filename": false},
		"archive_file":         {"output_path": true, "source_file": false, "source_dir": false},
	},
}

const rehearsalComputedPath = "a variable or computed path is refused: the gate admits only path.module or path.root, alone or followed by a literal starting with /"

// pathCallProblems holds the path argument of every path function call
// in expr to the path rule, and gates the template a templatefile names.
func (m rehearsalModule) pathCallProblems(at string, expr hclsyntax.Expression) []string {
	var problems []string
	hclsyntax.VisitAll(expr, func(node hclsyntax.Node) hcl.Diagnostics {
		if call, ok := node.(*hclsyntax.FunctionCallExpr); ok && rehearsalPathFunctions[call.Name] {
			problems = append(problems, m.pathCallProblem(fmt.Sprintf("%s: %s()", at, call.Name), call)...)
		}
		return nil
	})
	return problems
}

func (m rehearsalModule) pathCallProblem(at string, call *hclsyntax.FunctionCallExpr) []string {
	if len(call.Args) == 0 {
		return []string{at + ": " + rehearsalComputedPath}
	}
	path, problem := m.resolvePath(call.Args[0])
	switch {
	case problem != "":
		return []string{at + ": " + problem}
	case call.Name == "fileset" && len(call.Args) > 1:
		if problem := rehearsalGlobProblem(call.Args[1]); problem != "" {
			return []string{at + ": " + problem}
		}
	case call.Name == "templatefile":
		return m.templateProblems(at, path)
	}
	return nil
}

// templateProblems parses the template a templatefile names and holds
// every call and path in it to the function and path rules. A template
// calling templatefile is refused rather than followed.
func (m rehearsalModule) templateProblems(at, path string) []string {
	name := m.in.display(path)
	src, problem := rehearsalRead(path, name)
	if problem != "" {
		return []string{at + ": template " + problem}
	}
	expr, diags := hclsyntax.ParseTemplate(src, name, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return []string{fmt.Sprintf("%s: template %s does not parse, so the gate cannot vouch for it: %s", at, name, diags.Error())}
	}
	at += ": template " + name
	if slices.Contains(layer3CallsIn(expr), "templatefile") {
		return []string{at + ": calls templatefile(): the gate does not follow a template into another"}
	}
	return m.callProblems(at, expr)
}

// pathArgProblems holds a block's path arguments to the path rule, and a
// written one to the written-path rule.
func (m rehearsalModule) pathArgProblems(file string, block *hclsyntax.Block) []string {
	var problems []string
	for name, written := range rehearsalPathArgs[block.Type][block.Labels[0]] {
		for _, attr := range rehearsalNestedAttrs(block.Body, name) {
			path, problem := m.resolvePath(attr.Expr)
			if problem == "" && written {
				problem = rehearsalWrittenPathProblem(m.in.display(path))
			}
			if problem != "" {
				problems = append(problems, rehearsalAt(file, attr.SrcRange, rehearsalAddress(block)+"."+name)+": "+problem)
			}
		}
	}
	return problems
}

// rehearsalNestedAttrs finds every attribute a dotted name names under
// body, through each nested block of the named type, a dynamic block's
// content included.
func rehearsalNestedAttrs(body *hclsyntax.Body, name string) []*hclsyntax.Attribute {
	blockType, rest, nested := strings.Cut(name, ".")
	if !nested {
		if attr, ok := body.Attributes[name]; ok {
			return []*hclsyntax.Attribute{attr}
		}
		return nil
	}
	var attrs []*hclsyntax.Attribute
	for _, inner := range body.Blocks {
		switch {
		case inner.Type == blockType:
			attrs = append(attrs, rehearsalNestedAttrs(inner.Body, rest)...)
		case inner.Type == "dynamic" && len(inner.Labels) == 1 && inner.Labels[0] == blockType:
			for _, content := range inner.Body.Blocks {
				if content.Type == "content" {
					attrs = append(attrs, rehearsalNestedAttrs(content.Body, rest)...)
				}
			}
		}
	}
	return attrs
}

// resolvePath resolves a checked path, or says why it is refused. It is
// admitted only as path.module or path.root, alone or followed by a
// literal starting with /, resolved lexically from that directory and
// inside the copy. A bare relative literal is refused: which directory
// tofu resolves it from is not pinned.
func (m rehearsalModule) resolvePath(expr hclsyntax.Expression) (string, string) {
	if wrap, ok := expr.(*hclsyntax.TemplateWrapExpr); ok {
		expr = wrap.Wrapped
	}
	var path string
	switch e := expr.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		base, ok := m.pathBase(e)
		if !ok {
			return "", rehearsalComputedPath
		}
		path = base
	case *hclsyntax.TemplateExpr:
		if lit, ok := rehearsalLiteral(e); ok {
			if filepath.IsAbs(lit) {
				return "", "an absolute path is refused"
			}
			return "", "a bare relative path is refused: which directory tofu resolves it from is not pinned"
		}
		if len(e.Parts) != 2 {
			return "", rehearsalComputedPath
		}
		base, ok := m.pathBase(e.Parts[0])
		lit, isLit := rehearsalLiteral(e.Parts[1])
		if !ok || !isLit || !strings.HasPrefix(lit, "/") {
			return "", rehearsalComputedPath
		}
		path = filepath.Join(base, lit)
	default:
		return "", rehearsalComputedPath
	}
	if rel, err := filepath.Rel(m.in.boundary, path); err != nil || !filepath.IsLocal(rel) {
		return "", "the path resolves outside the copy"
	}
	return path, ""
}

// pathBase is the directory path.module or path.root names.
func (m rehearsalModule) pathBase(expr hclsyntax.Expression) (string, bool) {
	traversal, ok := expr.(*hclsyntax.ScopeTraversalExpr)
	if !ok || len(traversal.Traversal) != 2 || traversal.Traversal.RootName() != "path" {
		return "", false
	}
	attr, ok := traversal.Traversal[1].(hcl.TraverseAttr)
	switch {
	case ok && attr.Name == "module":
		return m.dir, true
	case ok && attr.Name == "root":
		return m.in.root, true
	}
	return "", false
}

// rehearsalLiteral is a string literal's value.
func rehearsalLiteral(expr hclsyntax.Expression) (string, bool) {
	switch e := expr.(type) {
	case *hclsyntax.TemplateExpr:
		if !e.IsStringLiteral() {
			return "", false
		}
	case *hclsyntax.LiteralValueExpr:
	default:
		return "", false
	}
	val, diags := expr.Value(nil)
	if diags.HasErrors() || !val.IsKnown() || val.IsNull() || !val.Type().Equals(cty.String) {
		return "", false
	}
	return val.AsString(), true
}

// rehearsalGlobProblem is why a fileset glob is refused, or "". It must be
// a relative literal with no .. element: any .., not only an escaping one,
// because ** can match zero directories. Backslashes are dropped before
// the check, since a glob reads one as an escape.
func rehearsalGlobProblem(expr hclsyntax.Expression) string {
	glob, ok := rehearsalLiteral(expr)
	switch {
	case !ok:
		return "the glob must be a literal"
	case filepath.IsAbs(glob):
		return "an absolute glob is refused"
	case slices.Contains(strings.Split(strings.ReplaceAll(glob, `\`, ""), "/"), ".."):
		return "a glob holding a .. element is refused"
	}
	return ""
}

// rehearsalWrittenPathProblem is why a written path, relative to the copy,
// is refused, or "": it names a file tofu could load, so an apply could
// write configuration every later step loads ungated.
func rehearsalWrittenPathProblem(rel string) string {
	elems := strings.Split(strings.ToLower(filepath.ToSlash(rel)), "/")
	base := elems[len(elems)-1]
	if slices.Contains(elems, "terraform.d") || slices.Contains(elems, ".terraform") || base == ".terraform.lock.hcl" {
		return "a written path is refused: tofu reads it"
	}
	for _, suffix := range append([]string{".tf", ".tfvars"}, rehearsalRefusedSuffixes...) {
		if strings.HasSuffix(base, suffix) {
			return fmt.Sprintf("a written path ending %s is refused: tofu could load it", suffix)
		}
	}
	return ""
}
