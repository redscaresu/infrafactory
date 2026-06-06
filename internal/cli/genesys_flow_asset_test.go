package cli

import "testing"

// S122 regression. The genesyscloud_flow resource reads `filepath` at
// PLAN time via the provider's CustomizeDiff. Without harness pre-
// placement, every architect-flow + full-stack sweep iteration hits
// "could not open <file>: no such file or directory" before any
// resource creates.

func TestEnsureGenesysFlowAsset_AddsStubWhenResourcePresent(t *testing.T) {
	files := map[string][]byte{
		"main.tf": []byte(`resource "genesyscloud_flow" "ivr" {
  filepath = "${path.module}/flow.yaml"
}
`),
	}
	ensureGenesysFlowAsset(files)
	if _, ok := files["flow.yaml"]; !ok {
		t.Fatalf("expected flow.yaml to be added; got files=%v", keysOf(files))
	}
}

func TestEnsureGenesysFlowAsset_NoopWhenNoFlowResource(t *testing.T) {
	files := map[string][]byte{
		"main.tf": []byte(`resource "genesyscloud_user" "u" {
  email = "x@y.z"
}
`),
	}
	ensureGenesysFlowAsset(files)
	if _, ok := files["flow.yaml"]; ok {
		t.Fatalf("flow.yaml should not be added when no genesyscloud_flow is declared")
	}
}

func TestEnsureGenesysFlowAsset_KeepsLLMProvidedAsset(t *testing.T) {
	custom := []byte("inboundCall:\n  name: custom\n")
	files := map[string][]byte{
		"main.tf":   []byte(`resource "genesyscloud_flow" "ivr" {}`),
		"flow.yaml": custom,
	}
	ensureGenesysFlowAsset(files)
	if string(files["flow.yaml"]) != string(custom) {
		t.Fatalf("ensureGenesysFlowAsset overwrote LLM-provided flow.yaml")
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
