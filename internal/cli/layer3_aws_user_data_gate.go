package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// The boot leg of the AWS gate. The instance boots a script infrafactory
// renders, never one the configuration chooses, from an image this run
// resolved, never one the configuration names.

// awsResolvedAMI is the AMI this run resolved and its root mapping as
// DescribeImages reported it.
type awsResolvedAMI struct {
	ID   string
	Root harness.AWSAMIRoot
}

const (
	awsMaxRootGiB     = 20
	awsRootVolumeType = "gp3"
)

// awsUserDataExempt is the predicate given to layer3FunctionCallProblems:
// the one file() call the AWS gate admits. It reads the rendered script,
// which awsUserDataFileProblem requires to be byte-equal to this run's
// render, so it can put nothing else in the instance's boot script. Every
// other call, and this call anywhere but user_data on an aws_instance,
// stays refused.
func awsUserDataExempt(block *hclsyntax.Block, attr *hclsyntax.Attribute) bool {
	return block.Type == "resource" && len(block.Labels) > 0 && block.Labels[0] == "aws_instance" &&
		attr.Name == "user_data" && awsIsUserDataExpr(attr.Expr)
}

// awsIsUserDataExpr matches the parse of generator.AWSUserDataLine's value
// and nothing else: file() by that exact name (core::file is another name),
// one argument, no expansion, and a template of exactly path.module and the
// literal "/infrafactory-user-data.sh". path.root, path.cwd, "..", a heredoc
// (whose literal ends in "\n") and any wrapping call or template all fail.
func awsIsUserDataExpr(expr hclsyntax.Expression) bool {
	call, ok := expr.(*hclsyntax.FunctionCallExpr)
	if !ok || call.Name != "file" || len(call.Args) != 1 || call.ExpandFinal {
		return false
	}
	tmpl, ok := call.Args[0].(*hclsyntax.TemplateExpr)
	if !ok || len(tmpl.Parts) != 2 {
		return false
	}
	scope, ok := tmpl.Parts[0].(*hclsyntax.ScopeTraversalExpr)
	if !ok || len(scope.Traversal) != 2 || scope.Traversal.RootName() != "path" {
		return false
	}
	if attr, ok := scope.Traversal[1].(hcl.TraverseAttr); !ok || attr.Name != "module" {
		return false
	}
	lit, ok := tmpl.Parts[1].(*hclsyntax.LiteralValueExpr)
	return ok && lit.Val.RawEquals(cty.StringVal("/"+generator.AWSUserDataFile))
}

// awsInstanceUserDataProblems requires an aws_instance to boot the rendered
// script: user_data present, and exactly generator.AWSUserDataLine.
func awsInstanceUserDataProblems(block *hclsyntax.Block, file string) []string {
	attr, ok := block.Body.Attributes["user_data"]
	if ok && awsIsUserDataExpr(attr.Expr) {
		return nil
	}
	return []string{fmt.Sprintf("%s: %s must set exactly `%s`; the instance boots only the script infrafactory renders",
		file, layer3BlockName(block), generator.AWSUserDataLine)}
}

// awsAMIProblems requires ami to be a literal equal to the id this run
// resolved. A variable, a lookup or another id would boot an image the
// root-volume bound was never checked against.
func awsAMIProblems(block *hclsyntax.Block, file string, resolved awsResolvedAMI) []string {
	if resolved.ID == "" {
		return []string{fmt.Sprintf("%s: %s cannot be checked: no AMI was resolved for this run", file, layer3BlockName(block))}
	}
	if attr, ok := block.Body.Attributes["ami"]; ok {
		// Value(nil) evaluates only constants: var.x, local.x and any call
		// fail it, so a match is a literal.
		if val, diags := attr.Expr.Value(nil); !diags.HasErrors() && val.RawEquals(cty.StringVal(resolved.ID)) {
			return nil
		}
	}
	return []string{fmt.Sprintf("%s: %s must set ami = %q, the AMI resolved for this run, as a literal",
		file, layer3BlockName(block), resolved.ID)}
}

// awsAMIRootProblems bounds the resolved image's own root volume. An
// aws_instance may omit root_block_device, and then launches with the
// image's mapping, so that mapping is what gets checked, every run. The
// block is not required instead: fakeaws models no block devices, so a
// required block would drift at Layer 2
// (TestFakeAWSStateCarriesNoRootBlockDevice).
func awsAMIRootProblems(resolved awsResolvedAMI) []string {
	root := resolved.Root
	var problems []string
	if root.SizeGiB < 1 || root.SizeGiB > awsMaxRootGiB {
		problems = append(problems, fmt.Sprintf("AMI %s's root volume is %d GiB; the gate permits 1 to %d", resolved.ID, root.SizeGiB, awsMaxRootGiB))
	}
	if root.VolumeType != awsRootVolumeType {
		problems = append(problems, fmt.Sprintf("AMI %s's root volume type is %q; the gate permits only %q", resolved.ID, root.VolumeType, awsRootVolumeType))
	}
	if !root.DeleteOnTermination {
		problems = append(problems, fmt.Sprintf("AMI %s's root volume is not deleted on termination, so destroying the instance would leave it billing", resolved.ID))
	}
	return problems
}

// awsUserDataFileProblem requires the script file() will read to be a
// regular file holding exactly this run's render. Lstat, not Stat: a
// committed symlink to /proc/self/environ passes every HCL check above.
func awsUserDataFileProblem(outputDir string, rendered []byte) string {
	name := generator.AWSUserDataFile
	if len(rendered) == 0 {
		return fmt.Sprintf("%s: no script was rendered for this run, so there is nothing to compare it with", name)
	}
	path := filepath.Join(outputDir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Sprintf("%s: cannot stat it, so cannot vouch for it", name)
	}
	if !info.Mode().IsRegular() {
		return fmt.Sprintf("%s: is not a regular file; a symlink or a directory is refused", name)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("%s: cannot read it, so cannot vouch for it", name)
	}
	if !bytes.Equal(got, rendered) {
		return fmt.Sprintf("%s: differs from the script rendered for this run", name)
	}
	return ""
}

// layer3BlockName renders `resource "t" "n"` as "t n".
func layer3BlockName(block *hclsyntax.Block) string {
	if len(block.Labels) < 2 {
		return block.Type
	}
	return block.Labels[0] + " " + block.Labels[1]
}
