package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// Layer 3 applies HCL that can arrive from a pull request (the S144 gate
// stages committed fixtures), in a process holding live cloud credentials.
// That is untrusted input, and it is validated by PARSING rather than by
// pattern matching.
//
// Regexes were tried and are not sound here. `resource /*x*/ "scaleway_k8s_cluster"`
// is valid HCL that a `resource\s+"` pattern misses, and no amount of
// patching the expression fixes the class of problem: the grammar permits
// comments and whitespace almost anywhere, so a scanner that is not a
// parser will always have a gap.
//
// The policy is deny-by-default on BLOCK TYPE, not a denylist of known
// escape hatches. Denylists fail the same way regexes do — `module` and
// `provisioner` were blocked, and `data "external"` with `program = [...]`
// still executed commands during plan. Enumerating what is permitted ends
// that game.
var layer3AllowedTopLevelBlocks = map[string]bool{
	"terraform": true,
	"provider":  true,
	"variable":  true,
	"output":    true,
	"locals":    true,
	"resource":  true,
	"data":      true,
}

// layer3DeniedNestedBlocks execute commands during apply, inside a process
// that holds the cloud credentials.
var layer3DeniedNestedBlocks = map[string]bool{
	"provisioner": true,
	"connection":  true,
}

// validateLayer3HCLShape refuses anything a Layer 3 stack has no business
// containing, before any tofu invocation.
//
// Before, not during: `data "external"` runs its program at PLAN time, so a
// check that waits for plan output has already lost.
func validateLayer3HCLShape(outputDir string, allowedResourceTypes []string) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return fmt.Errorf("read output directory for layer 3 HCL validation: %w", err)
	}
	problems := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}
		path := filepath.Join(outputDir, entry.Name())
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", entry.Name(), readErr)
		}
		file, diags := hclsyntax.ParseConfig(src, entry.Name(), hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			// Unparseable means unknowable. Fail closed rather than apply it.
			return fmt.Errorf("layer 3 refuses %s: cannot parse it, so cannot vouch for it (%s)", entry.Name(), diags.Error())
		}
		body, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			return fmt.Errorf("layer 3 refuses %s: unexpected body type", entry.Name())
		}
		problems = append(problems, layer3BlockProblems(body, entry.Name(), allowedResourceTypes)...)
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("layer 3 refuses this configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}

func layer3BlockProblems(body *hclsyntax.Body, file string, allowedResourceTypes []string) []string {
	problems := make([]string, 0)
	for _, block := range body.Blocks {
		switch {
		case !layer3AllowedTopLevelBlocks[block.Type]:
			problems = append(problems, fmt.Sprintf("%s: %q blocks are not permitted", file, block.Type))
			continue
		case block.Type == "resource":
			if len(block.Labels) > 0 && !resourceTypeAllowed(block.Labels[0], allowedResourceTypes) {
				problems = append(problems, fmt.Sprintf("%s: resource type %q is not in allow_resource_types", file, block.Labels[0]))
			}
		case block.Type == "data":
			// `data "external"` runs a program at plan time. Restricting
			// data sources to the cloud under test removes that whole
			// family rather than naming the dangerous ones.
			if len(block.Labels) > 0 && !strings.HasPrefix(block.Labels[0], "scaleway_") {
				problems = append(problems, fmt.Sprintf("%s: data source %q is not a scaleway_ source", file, block.Labels[0]))
			}
		case block.Type == "provider":
			if len(block.Labels) > 0 && block.Labels[0] != "scaleway" {
				problems = append(problems, fmt.Sprintf("%s: provider %q is not permitted", file, block.Labels[0]))
			}
		}
		problems = append(problems, layer3NestedProblems(block, file)...)
	}
	return problems
}

// layer3NestedProblems walks the whole tree: a provisioner is nested inside
// a resource, not top level.
func layer3NestedProblems(block *hclsyntax.Block, file string) []string {
	problems := make([]string, 0)
	if block.Body == nil {
		return problems
	}
	for _, nested := range block.Body.Blocks {
		if layer3DeniedNestedBlocks[nested.Type] {
			problems = append(problems, fmt.Sprintf("%s: %q executes commands during apply, in a process holding cloud credentials", file, nested.Type))
		}
		problems = append(problems, layer3NestedProblems(nested, file)...)
	}
	return problems
}
