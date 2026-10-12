package cli

import (
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// rehearsalAWSProviderProblems holds a root provider "aws" block to the
// aws argument rule (ADR-0042 § provider "aws"). Closed: every argument
// but alias, and every nested block, is refused.
func rehearsalAWSProviderProblems(file string, block *hclsyntax.Block) []string {
	const refused = `: refused: a provider "aws" block admits only alias`
	var problems []string
	for name, attr := range block.Body.Attributes {
		if name != "alias" {
			problems = append(problems, rehearsalAt(file, attr.SrcRange, rehearsalAddress(block)+"."+name)+refused)
		}
	}
	for _, inner := range block.Body.Blocks {
		problems = append(problems, rehearsalAt(file, inner.DefRange(), rehearsalAddress(block)+"."+inner.Type)+refused)
	}
	return problems
}
