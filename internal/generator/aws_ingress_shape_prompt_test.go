package generator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const awsInlineIngressSentence = "Open each port the service needs with an inline `ingress` block inside its `aws_security_group`, and write no `aws_security_group_rule` or `aws_vpc_security_group_ingress_rule` resources: inline blocks are the form this validation environment applies end to end."

// A service scenario is told to open ports with inline ingress blocks, the
// form TestE2E_AWSWebStepOne applies. Scenarios without a service: keep their
// prompt unchanged, so they may still write standalone rules.
func TestPhase2PromptOpensServicePortsInline(t *testing.T) {
	root := findRepoRoot(t)
	awsInstance, err := os.ReadFile(filepath.Join(root, "scenarios", "training", "aws-instance.yaml"))
	require.NoError(t, err)

	for name, render := range phase2Renderers(t, root) {
		t.Run(name, func(t *testing.T) {
			noService := Request{Cloud: "aws", ScenarioYAML: awsInstance}
			prompt, err := render(noService)
			require.NoError(t, err)
			assert.NotContains(t, prompt, awsInlineIngressSentence)
			assert.NotContains(t, prompt, "aws_security_group_rule")
			assert.NotContains(t, prompt, "aws_vpc_security_group_ingress_rule")

			withService := noService
			withService.UserDataLine = AWSUserDataLine
			prompt, err = render(withService)
			require.NoError(t, err)
			assert.Contains(t, prompt, awsInlineIngressSentence)
		})
	}
}

// The sentence's reason holds only while the HCL CI applies uses the shape it
// prescribes.
func TestWebStepOneOpensPortsInline(t *testing.T) {
	root := findRepoRoot(t)
	path := filepath.Join(root, "internal", "e2e", "testdata", "aws-web-step-one", "web-step-one.tf")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	file, diags := hclsyntax.ParseConfig(content, path, hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())

	groups := 0
	for _, block := range file.Body.(*hclsyntax.Body).Blocks {
		if block.Type != "resource" || len(block.Labels) < 2 {
			continue
		}
		switch block.Labels[0] {
		case "aws_security_group_rule", "aws_vpc_security_group_ingress_rule":
			assert.Failf(t, "standalone rule", "%s.%s", block.Labels[0], block.Labels[1])
		case "aws_security_group":
			groups++
			assert.NotContains(t, block.Body.Attributes, "ingress", "%s uses the ingress attribute", block.Labels[1])
			ingress := 0
			for _, nested := range block.Body.Blocks {
				if nested.Type == "ingress" {
					ingress++
				}
			}
			assert.Positive(t, ingress, "%s has no inline ingress block", block.Labels[1])
		}
	}
	assert.Positive(t, groups, "no aws_security_group in %s", path)
}
