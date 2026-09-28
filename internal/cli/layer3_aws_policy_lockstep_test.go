package cli

import (
	"context"
	"encoding/json"
	"maps"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/redscaresu/infrafactory/internal/harness"
)

const awsVPCRequiredPolicy = "../../policies/aws/vpc_required.rego"

// vpc_required.rego demands an attribute of every aws_instance, and the gate
// admits only the attributes on aws_instance's table entry. If the two
// disagree, no generated instance can pass both. Both sides are checked by
// behaviour: the demand comes from evaluating the policy, and the admitted
// instance is the one validateAWSLayer3HCLShape admits.
func TestAWSVPCRequiredAgreesWithLayer3Gate(t *testing.T) {
	instance := awsAdmittedInstance(t)
	expressions := make(map[string]any, len(instance.Body.Attributes))
	for name, attr := range instance.Body.Attributes {
		expressions[name] = awsPlanExpression(t, attr.Expr)
	}
	assert.Empty(t, awsVPCRequiredDenials(t, expressions), "the admitted aws_instance passes vpc_required")

	denials := awsVPCRequiredDenials(t, map[string]any{})
	require.Len(t, denials, 1, "an aws_instance that configures nothing must produce exactly one denial")
	demand := regexp.MustCompile(`^aws_instance\.web has no ([a-z_]+) `).FindStringSubmatch(denials[0])
	require.Len(t, demand, 2, "the denial no longer names the attribute it demands: %s", denials[0])
	assert.Equal(t, "subnet_id", demand[1])

	assert.Empty(t, awsTableLacks(awsAttrAllowlist, "aws_instance", demand[1]))

	dropped := maps.Clone(awsAttrAllowlist)
	dropped["aws_instance"] = maps.Clone(dropped["aws_instance"])
	delete(dropped["aws_instance"], "subnet_id")
	assert.NotEmpty(t, awsTableLacks(dropped, "aws_instance", demand[1]), "dropping subnet_id from the table must fail this test")
}

// awsTableLacks reports a policy-demanded attribute the gate's table does
// not admit on resourceType.
func awsTableLacks(table map[string]map[string]awsRule, resourceType, attr string) string {
	if rule, ok := table[resourceType][attr]; ok && rule.kind != awsBlock {
		return ""
	}
	return "vpc_required.rego demands " + attr + " of every " + resourceType + ", and the gate's attribute table does not admit it"
}

func awsAdmittedInstance(t *testing.T) *hclsyntax.Block {
	t.Helper()
	for _, block := range parseAWSStack(t, awsAdmittedStack(t)["main.tf"]).Blocks {
		if block.Type == "resource" && block.Labels[0] == "aws_instance" {
			return block
		}
	}
	require.FailNow(t, "the admitted stack has no aws_instance")
	return nil
}

// awsPlanExpression renders expr as `tofu show -json` does in
// configuration.expressions: a constant, or the references it makes.
func awsPlanExpression(t *testing.T, expr hclsyntax.Expression) map[string]any {
	t.Helper()
	if val, diags := expr.Value(nil); !diags.HasErrors() {
		encoded, err := ctyjson.Marshal(val, val.Type())
		require.NoError(t, err)
		return map[string]any{"constant_value": json.RawMessage(encoded)}
	}
	refs := make([]string, 0)
	for _, traversal := range expr.Variables() {
		refs = append(refs, strings.Join(awsTraversalText(traversal), "."))
	}
	return map[string]any{"references": refs}
}

// awsVPCRequiredDenials evaluates the policy against a plan holding one
// aws_instance configured with expressions.
func awsVPCRequiredDenials(t *testing.T, expressions map[string]any) []string {
	t.Helper()
	const address = "aws_instance.web"
	plan, err := json.Marshal(map[string]any{
		"planned_values": map[string]any{"root_module": map[string]any{"resources": []any{
			map[string]any{"address": address, "mode": "managed", "type": "aws_instance", "values": map[string]any{}},
		}}},
		"configuration": map[string]any{"root_module": map[string]any{"resources": []any{
			map[string]any{"address": address, "mode": "managed", "type": "aws_instance", "expressions": expressions},
		}}},
	})
	require.NoError(t, err)
	failures, err := harness.EvaluatePlanPolicies(context.Background(), plan, []string{awsVPCRequiredPolicy})
	require.NoError(t, err)
	denials := make([]string, 0, len(failures))
	for _, failure := range failures {
		denials = append(denials, failure.Detail)
	}
	return denials
}
