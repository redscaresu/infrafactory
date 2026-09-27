package cli

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	vpcRequiredPolicy = "../../policies/scaleway/vpc_required.rego"
	vpcDeniedServer   = "scaleway_instance_server.web"
)

// vpc_required.rego's denial is repair-loop input, and it tells the generator
// which resource types the Layer 3 gate refuses. That claim went stale in three
// places before this test existed. Both sides are checked by behaviour: the
// denial comes from evaluating the policy, and each claim is put to the gate as
// a real HCL block.
//
//   - A type named in the denial's prose (outside backticks) is a claim that the gate refuses it.
//   - A type the policy handles that the gate refuses must be named, or the
//     generator is steered into a refusal it was never told about.
//   - The code span the denial prescribes must pass the whole shape gate.
func TestVPCRequiredDenialAgreesWithLayer3Gate(t *testing.T) {
	denial := vpcRequiredDenial(t)
	// The denied server's own address is the subject, not a claim.
	prose, code := splitCodeSpans(strings.ReplaceAll(denial, vpcDeniedServer, ""))

	claimed := scalewayTypesIn(prose)
	require.NotEmpty(t, claimed, "the denial no longer names a refused type; if that is deliberate, delete this test")
	for _, typ := range claimed {
		assert.NotEmpty(t, layer3RefusalOf(t, typ),
			"vpc_required.rego tells the generator the Layer 3 gate refuses %s, and it does not:\n%s", typ, denial)
	}

	src, err := os.ReadFile(vpcRequiredPolicy)
	require.NoError(t, err)
	for _, typ := range scalewayTypesIn(string(src)) {
		if len(layer3RefusalOf(t, typ)) == 0 {
			continue
		}
		assert.Contains(t, claimed, typ,
			"the Layer 3 gate refuses %s, which vpc_required.rego handles, but its denial does not say so:\n%s", typ, denial)
	}

	require.Len(t, code, 1, "the denial should prescribe exactly one code span: %s", denial)
	dir := writeShapeHCL(t, shapeProject+fmt.Sprintf(`
resource "scaleway_vpc_private_network" "main" { name = "pn" }
resource "scaleway_instance_server" "web" {
  name = "web"
  %s
}`, strings.ReplaceAll(code[0], "NAME", "main")))
	assert.NoError(t, validateLayer3HCLShape(dir, append(gateAllowlist,
		"scaleway_vpc_private_network", "scaleway_instance_server")),
		"the Layer 3 gate refuses the attachment vpc_required.rego prescribes: %s", code[0])
}

// vpcRequiredDenial evaluates the policy against a plan with one unattached
// server and returns the single message it produces.
func vpcRequiredDenial(t *testing.T) string {
	t.Helper()
	plan := []byte(fmt.Sprintf(`{
  "planned_values": {"root_module": {"resources": [
    {"address": %[1]q, "type": "scaleway_instance_server"}
  ]}},
  "configuration": {"root_module": {"resources": [
    {"address": %[1]q, "type": "scaleway_instance_server", "expressions": {}}
  ]}}
}`, vpcDeniedServer))
	failures, err := harness.EvaluatePlanPolicies(context.Background(), plan, []string{vpcRequiredPolicy})
	require.NoError(t, err)
	require.Len(t, failures, 1, "an unattached server must produce exactly one denial")
	return failures[0].Detail
}

var backtickSpanRe = regexp.MustCompile("`([^`]*)`")

// splitCodeSpans returns the text outside backticks and each span inside them.
func splitCodeSpans(s string) (string, []string) {
	var code []string
	for _, m := range backtickSpanRe.FindAllStringSubmatch(s, -1) {
		code = append(code, m[1])
	}
	return backtickSpanRe.ReplaceAllString(s, " "), code
}

// scalewayTypesIn returns the distinct resource types named in s. A trailing
// `.` is an attribute or name traversal, so the type ends there.
func scalewayTypesIn(s string) []string {
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`\bscaleway_[a-z0-9_]+`).FindAllString(s, -1) {
		seen[m] = true
	}
	out := make([]string, 0, len(seen))
	for typ := range seen {
		out = append(out, typ)
	}
	sort.Strings(out)
	return out
}

// layer3RefusalOf parses an empty resource of the given type and asks the gate
// whether it refuses it.
func layer3RefusalOf(t *testing.T, typ string) []string {
	t.Helper()
	file, diags := hclsyntax.ParseConfig([]byte(fmt.Sprintf("resource %q \"x\" {}\n", typ)), "main.tf", hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())
	body, ok := file.Body.(*hclsyntax.Body)
	require.True(t, ok)
	require.Len(t, body.Blocks, 1)
	return layer3UndestroyableResourceProblems(body.Blocks[0], "main.tf")
}
