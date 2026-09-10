package generator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The refusal that cost three of five iterations in run 20260910T104418Z,
// verbatim from that run's log.
func TestExtractGatePitfallLearnsTheRefusedInstanceType(t *testing.T) {
	detail := `layer 3 refuses this configuration: compute.tf: scaleway_instance_server web sets type to "PLAY2-NANO"; the gate permits only [DEV1-S DEV1-M]`

	got := ExtractGatePitfall(detail, "web-live-paris")

	require.NotNil(t, got)
	assert.Equal(t, "scaleway_instance_server", got.Resource)
	assert.Equal(t, "fix", got.Source, "a refusal names the permitted values, so it is a remedy and not a symptom")
	assert.Equal(t, "web-live-paris", got.DiscoveredFrom)
	assert.Contains(t, got.Rule, "the gate permits only [DEV1-S DEV1-M]")
	assert.NotContains(t, got.Rule, "compute.tf", "the filename is where it was found, not what to do")
}

func TestExtractGatePitfallLearnsTheUndestroyableNIC(t *testing.T) {
	detail := "layer 3 refuses this configuration: network.tf: `scaleway_instance_private_nic` is refused -- a private NIC is deletable only while its server is powered off. Declare the attachment on the server instead: `private_network { pn_id = scaleway_vpc_private_network.NAME.id }`"

	got := ExtractGatePitfall(detail, "web-live-paris")

	require.NotNil(t, got)
	assert.Equal(t, "scaleway_instance_private_nic", got.Resource)
	assert.Contains(t, got.Rule, "private_network { pn_id =", "the remedy is the reason to keep the text verbatim")
}

// Provider stderr keeps going to ExtractDescriptivePitfall. Claiming it
// here would tag a symptom as a remedy, which is the mislabelling the
// source enum exists to prevent.
func TestExtractGatePitfallIgnoresProviderErrors(t *testing.T) {
	detail := "exit status 1 | stderr: Error: scaleway-sdk-go: resource with ID is not found"
	assert.Nil(t, ExtractGatePitfall(detail, "web-live-paris"))
}

// A rule filed under the wrong type is worse than no rule: it is offered
// to the generator every time that type appears.
func TestExtractGatePitfallSkipsRefusalsThatNameNoResourceType(t *testing.T) {
	detail := `layer 3 refuses this configuration: main.tf: provider setting "api_url" is not permitted`
	assert.Nil(t, ExtractGatePitfall(detail, "web-live-paris"))
}

// The gate joins independent problems with "; ". Keeping both would file
// a rule under one type that talks about another.
func TestExtractGatePitfallKeepsOnlyTheFirstProblem(t *testing.T) {
	detail := `layer 3 refuses this configuration: compute.tf: scaleway_instance_server web sets type to "PLAY2-NANO"; the gate permits only [DEV1-S]; network.tf: scaleway_instance_private_nic cannot be destroyed`

	got := ExtractGatePitfall(detail, "s")

	require.NotNil(t, got)
	assert.Equal(t, "scaleway_instance_server", got.Resource)
	assert.NotContains(t, got.Rule, "private_nic", "a second, unrelated refusal must not ride along")
	assert.Contains(t, got.Rule, "the gate permits only [DEV1-S]",
		"but a semicolon INSIDE one refusal must not truncate it")
}

// A run failure detail is `<err> | stderr: <stderr>`, and for a gate
// refusal both halves are the same sentence. Taking everything after the
// prefix produced the message followed by a copy of itself, filename and
// all -- which is what the wiring test caught and this one pins.
func TestExtractGatePitfallDropsTheDuplicatedStderrTail(t *testing.T) {
	msg := `scaleway_instance_server web sets type to "PLAY2-NANO"; the gate permits only [DEV1-S DEV1-M]`
	detail := "layer 3 refuses this configuration: compute.tf: " + msg +
		" | stderr: layer 3 refuses this configuration: compute.tf: " + msg

	got := ExtractGatePitfall(detail, "web-live-paris")

	require.NotNil(t, got)
	assert.Equal(t, msg, got.Rule, "the rule is the message once, not twice")
	assert.NotContains(t, got.Rule, "stderr")
	assert.NotContains(t, got.Rule, "compute.tf")
}

// Recording the refusal verbatim is only safe if the refusal is true.
// The NIC message used to end "...which is removed with the server",
// which ADR-0029's refutation killed -- and a verbatim pitfall would
// have made that wrong claim durable. Pinned here because the property
// belongs to the extractor's contract, not just to that one message.
func TestExtractGatePitfallDoesNotPromiseTheInlineBlockDestroysCleanly(t *testing.T) {
	detail := "layer 3 refuses this configuration: network.tf: `scaleway_instance_private_nic` is refused -- a private NIC is deletable only while its server is powered off. Declare the attachment on the server instead: `private_network { pn_id = scaleway_vpc_private_network.NAME.id }`"

	got := ExtractGatePitfall(detail, "web-live-paris")

	require.NotNil(t, got)
	assert.NotContains(t, got.Rule, "removed with the server",
		"teardown works because the harness powers the server off (ADR-0030), not because of the HCL shape")
}

// The other Layer 3 refusal, and the one a generator hits most: reaching
// for a resource type nobody budgeted for. It has no "<file>: " part, so
// matching only the shape-gate prefix dropped the whole class.
func TestExtractGatePitfallLearnsTheAllowlistRefusal(t *testing.T) {
	detail := "layer 3 refuses to apply resource type(s) scaleway_k8s_cluster: not in validation.layers.sandbox_deploy.allow_resource_types (scaleway_lb*, scaleway_instance_server). These types are denied by default because they are slow and costly to provision against real Scaleway; either use a cheaper equivalent or widen the allowlist deliberately"

	got := ExtractGatePitfall(detail, "k8s-cluster-paris")

	require.NotNil(t, got)
	assert.Equal(t, "scaleway_k8s_cluster", got.Resource)
	assert.Equal(t, "fix", got.Source)
	assert.Contains(t, got.Rule, "denied by default",
		"the reason is kept: without it the rule reads as a fact about the type rather than a cost decision this repository made")
}

// A pitfall has one resource key. A plural allowlist refusal would file
// a rule naming several types under whichever came first -- offered to
// the generator whenever that type appears, talking about another.
func TestExtractGatePitfallSkipsAPluralAllowlistRefusal(t *testing.T) {
	detail := "layer 3 refuses to apply resource type(s) scaleway_k8s_cluster, scaleway_rdb_instance: not in validation.layers.sandbox_deploy.allow_resource_types (scaleway_lb*)"
	assert.Nil(t, ExtractGatePitfall(detail, "full-stack-paris"))
}

// The hardcoded VPC fallback must agree with what the repository
// actually enforces. It prescribed a standalone
// `scaleway_instance_private_nic` until 2026-09-10, by which time the
// Layer 3 gate refused that resource, `vpc_required`'s denial argued
// against it, and a real teardown could not delete one -- and the
// learning loop recorded it as a fresh pitfall anyway, in the same run
// whose Layer 1 was saying the opposite.
//
// This is an audit rather than a spelling check: a prescriptive rule
// frozen in Go cannot be reviewed next to the code that enforces it, so
// the only defence is a test that fails when they diverge.
func TestVPCFallbackPrescribesTheShapeTheGateAccepts(t *testing.T) {
	got := ExtractDescriptivePitfall(
		"scaleway_instance_server.web is not attached to a private network via scaleway_instance_private_nic", "s")
	require.NotNil(t, got, "the VPC fallback must still fire for the policy's own wording")

	assert.Contains(t, got.Rule, "private_network { pn_id =",
		"the fallback must prescribe the inline attachment")
	assert.NotContains(t, got.Rule, "The private NIC has the shape",
		"it must not hand the generator a standalone NIC the Layer 3 gate refuses")
}
