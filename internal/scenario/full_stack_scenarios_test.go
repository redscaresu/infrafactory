package scenario_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/redscaresu/infrafactory/internal/scenario"
)

// TestFullStackScenariosLoad locks the per-cloud full-stack training
// scenarios to the scenario loader + JSON schema. The full-stack
// scenarios are the headline composition examples for each cloud
// (drive the README quickstart and act as the integration target for
// every per-service handler in the matching fake-cloud mock), so a
// silent schema regression in any one of them would break the
// onboarding path before the user sees an error.
//
// What this catches:
//   - YAML syntax errors / typos in any full-stack scenario.
//   - scenario.schema.json drift that rejects a previously valid
//     resource block (we add a new resource key to one full-stack
//     scenario, forget to update the schema, the test fails).
//
// What this does NOT catch (out of scope — these need a tofu drive
// against the matching mock, which the per-cloud
// runGCPServiceScenario / runScalewayServiceScenario helpers cover
// for the per-service subset):
//   - whether the LLM can generate working HCL for the full stack.
//   - whether the fake-cloud handlers actually compose end-to-end
//     under a single tofu apply (the per-service e2e tests cover
//     each resource family in isolation).
func TestFullStackScenariosLoad(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	scenariosDir := filepath.Join(repoRoot, "scenarios", "training")
	schemaPath := filepath.Join(repoRoot, "scenario.schema.json")

	cases := []struct {
		name  string
		file  string
		cloud string
	}{
		{"scaleway", "full-stack-paris.yaml", "scaleway"},
		{"gcp", "gcp-full-stack.yaml", "gcp"},
		{"aws", "aws-full-stack.yaml", "aws"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc, err := scenario.LoadWithSchema(filepath.Join(scenariosDir, tc.file), schemaPath)
			if err != nil {
				t.Fatalf("LoadWithSchema(%s): %v", tc.file, err)
			}
			if sc.Cloud != tc.cloud {
				t.Errorf("cloud: got %q want %q", sc.Cloud, tc.cloud)
			}
			if len(sc.AcceptanceCriteria) == 0 {
				t.Errorf("%s: expected at least one acceptance criterion", tc.file)
			}
		})
	}
}
