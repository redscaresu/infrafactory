package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// livePartialApplyState is what tofu leaves on disk when an apply creates
// the project, then dies before finishing: real resources, recorded.
const livePartialApplyState = `{
  "version": 4,
  "resources": [
    {
      "type": "scaleway_account_project",
      "name": "main",
      "instances": [{"attributes": {"id": "11111111-1111-1111-1111-111111111111"}}]
    },
    {
      "type": "scaleway_lb_ip",
      "name": "main",
      "instances": [{"attributes": {
        "id": "fr-par-1/22222222-2222-2222-2222-222222222222",
        "project_id": "11111111-1111-1111-1111-111111111111"
      }}]
    }
  ]
}`

// writePartialLiveState plants that state in the output dir the run will
// actually use.
func writePartialLiveState(t *testing.T, outputDir string) {
	t.Helper()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("mkdir output dir: %v", err)
	}
	path := filepath.Join(outputDir, harness.LiveStateFilename)
	if err := os.WriteFile(path, []byte(livePartialApplyState), 0o600); err != nil {
		t.Fatalf("write live state: %v", err)
	}
}

// sandboxCleanupOpts builds a Layer 3 run whose sandbox apply behaves as
// the caller specifies, with everything else on the happy path.
func sandboxCleanupOpts(outputRoot string, sandboxDeploy *fakeSandboxDeployHarness, sandboxDestroy *fakeSandboxDestroyHarness, sweep *fakeOrphanSweep) runtimeOptions {
	return runtimeOptions{
		configLoader: func(path string) (config.Config, error) {
			cfg, err := config.Load(path)
			if err != nil {
				return config.Config{}, err
			}
			cfg.Validation.Layers.SandboxDeploy.Enabled = true
			// Without this the run uses the package-relative ./output and
			// can pass on state another test happened to leave behind.
			cfg.Paths.Output = outputRoot
			return cfg, nil
		},
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			MockDeploy: &fakeMockDeployHarness{
				result: &harness.MockDeployResult{
					Apply:         harness.StageResult{Stage: "apply"},
					StateSnapshot: []byte(`{}`),
				},
			},
			Destroy: &fakeDestroyHarness{
				result: &harness.DestroyResult{
					Destroy:       harness.StageResult{Stage: "destroy"},
					StateSnapshot: []byte(`{"instance":{"servers":[]}}`),
					OrphanCount:   0,
				},
			},
			SandboxDeploy:  sandboxDeploy,
			SandboxDestroy: sandboxDestroy,
			OrphanSweep:    sweep,
			RealProbe:      &fakeRealProbeHarness{result: &harness.RealProbeResult{}},
		},
	}
}

// The lb-paris canary leaked a real project and load-balancer IP exactly
// this way: `tofu apply` created the project and the LB IP, then died on
// an API permission error. Cleanup was gated on the apply having
// SUCCEEDED, so nothing was destroyed and nothing was swept, and the
// resources billed until they were reaped by hand.
//
// tofu creates resources one at a time, so a partial apply is the case
// that most needs cleanup, not least.
func TestFailedSandboxApplyStillDestroysAndSweeps(t *testing.T) {
	h := newCommandTestHarness(t)
	scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)
	sandboxCredsForTest(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")
	// The apply died partway and left these behind. This is the whole
	// premise of the test, so it is planted explicitly rather than
	// inherited from whatever ran before.
	writePartialLiveState(t, filepath.Join(outputRoot, "unsupported-dns"))

	sandboxDeploy := &fakeSandboxDeployHarness{
		err: &harness.SandboxDeployError{
			Stage: "apply",
			Apply: harness.StageResult{
				Stage:  "apply",
				Stderr: "Error: scaleway-sdk-go: insufficient permissions: read loadbalancer",
			},
			Err: errors.New("exit status 1"),
		},
	}
	sandboxDestroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}},
	}
	sweep := &fakeOrphanSweep{}

	cmd := newTestCommandForTest(sandboxCleanupOpts(outputRoot, sandboxDeploy, sandboxDestroy, sweep))
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected the test to fail when the sandbox apply fails")
	}

	if sandboxDestroy.calls != 1 {
		t.Errorf("got %d sandbox destroy calls after a failed apply, want 1 — real resources leak otherwise", sandboxDestroy.calls)
	}
	if sweep.calls != 1 {
		t.Errorf("got %d orphan sweep calls after a failed apply, want 1 — the leak would go unreported", sweep.calls)
	}
	if !strings.Contains(stdout.String(), "sandbox_deploy/destroy") {
		t.Errorf("no sandbox destroy stage reported:\n%s", stdout.String())
	}
	// The reason the apply died must survive into the report; reproducing
	// a real Layer 3 failure costs money.
	if !strings.Contains(stdout.String(), "insufficient permissions") {
		t.Errorf("failure detail lost the provider message:\n%s", stdout.String())
	}
}

// The fix must not change the happy path: exactly one destroy, one sweep.
func TestSuccessfulSandboxApplyStillDestroysOnce(t *testing.T) {
	h := newCommandTestHarness(t)
	scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)
	sandboxCredsForTest(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")
	writePartialLiveState(t, filepath.Join(outputRoot, "unsupported-dns"))

	sandboxDeploy := &fakeSandboxDeployHarness{
		result: &harness.SandboxDeployResult{
			Init:  harness.StageResult{Stage: "init"},
			Apply: harness.StageResult{Stage: "apply"},
		},
	}
	sandboxDestroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}},
	}
	sweep := &fakeOrphanSweep{}

	cmd := newTestCommandForTest(sandboxCleanupOpts(outputRoot, sandboxDeploy, sandboxDestroy, sweep))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected failure on the happy path: %v", err)
	}
	if sandboxDestroy.calls != 1 {
		t.Errorf("got %d sandbox destroy calls, want 1", sandboxDestroy.calls)
	}
	if sweep.calls != 1 {
		t.Errorf("got %d sweep calls, want 1", sweep.calls)
	}
}

// A failure before any resource exists must NOT trigger cleanup. An init
// or plan failure writes no live state, so destroying would tear down
// nothing and the sweep would then report that it could not verify --
// sending the operator to chase a leak that cannot exist.
func TestPreApplySandboxFailureSkipsCleanup(t *testing.T) {
	h := newCommandTestHarness(t)
	scenarioPath := writeUnsupportedCriteriaScenario(t, h.WorkspaceDir)
	sandboxCredsForTest(t)
	outputRoot := filepath.Join(h.WorkspaceDir, "output")
	// Deliberately no live state: nothing was created.

	sandboxDeploy := &fakeSandboxDeployHarness{
		err: &harness.SandboxDeployError{
			Stage: "plan",
			Plan:  harness.StageResult{Stage: "plan", Stderr: "Error: Invalid resource type"},
			Err:   errors.New("exit status 1"),
		},
	}
	sandboxDestroy := &fakeSandboxDestroyHarness{
		result: &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}},
	}
	sweep := &fakeOrphanSweep{}

	cmd := newTestCommandForTest(sandboxCleanupOpts(outputRoot, sandboxDeploy, sandboxDestroy, sweep))
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", h.ConfigPath})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected the test to fail when the sandbox plan fails")
	}
	if sandboxDestroy.calls != 0 {
		t.Errorf("got %d destroy calls with no live state, want 0", sandboxDestroy.calls)
	}
	if sweep.calls != 0 {
		t.Errorf("got %d sweep calls with no live state, want 0 — a spurious leak warning", sweep.calls)
	}
}
