package cli

import (
	"fmt"

	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// rehearsalPathFunctions read the path their first argument names
// (ADR-0042 § Paths and templates).
var rehearsalPathFunctions = map[string]bool{
	"file": true, "fileexists": true, "fileset": true, "filebase64": true,
	"filemd5": true, "filesha1": true, "filesha256": true, "filesha512": true,
	"filebase64sha256": true, "filebase64sha512": true, "templatefile": true,
}

// rehearsalPathArgs are the path arguments of the local and archive types,
// by block kind and type.
var rehearsalPathArgs = map[string]map[string][]string{
	"resource": {
		"local_file":           {"filename", "source"},
		"local_sensitive_file": {"filename", "source"},
		"archive_file":         {"output_path", "source_file", "source_dir"},
	},
	"data": {
		"local_file":           {"filename"},
		"local_sensitive_file": {"filename"},
		"archive_file":         {"output_path", "source_file", "source_dir"},
	},
}

const rehearsalPathsRefused = "the gate admits no path"

// rehearsalPathCallProblems holds every call to a path function to the
// path rule. Closed: each one is refused.
func rehearsalPathCallProblems(at string, expr hclsyntax.Expression) []string {
	var problems []string
	for _, fn := range layer3CallsIn(expr) {
		if rehearsalPathFunctions[fn] {
			problems = append(problems, fmt.Sprintf("%s: calls %s(), which reads a path: %s", at, fn, rehearsalPathsRefused))
		}
	}
	return problems
}

// rehearsalPathArgProblems holds a local or archive block's path arguments
// to the path rule. Closed: each one set is refused.
func rehearsalPathArgProblems(file string, block *hclsyntax.Block) []string {
	var problems []string
	for _, name := range rehearsalPathArgs[block.Type][block.Labels[0]] {
		if attr, ok := block.Body.Attributes[name]; ok {
			problems = append(problems, rehearsalAt(file, attr.SrcRange, rehearsalAddress(block)+"."+name)+": refused: "+rehearsalPathsRefused)
		}
	}
	return problems
}
