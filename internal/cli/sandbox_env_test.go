package cli

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// isolateSCWConfig points the scw-config lookup at an empty directory so
// the developer's real ~/.config/scw/config.yaml cannot influence a test.
// Without this, whether these tests pass depends on the machine.
func isolateSCWConfig(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func sandboxTestRuntime() *CommandRuntime {
	return &CommandRuntime{Config: config.Config{
		Scaleway: config.ScalewayConfig{Region: "fr-par", Zone: "fr-par-1"},
	}}
}

func sandboxTestRuntimeWithFallback(projectID string) *CommandRuntime {
	rt := sandboxTestRuntime()
	rt.Config.Scaleway.FallbackProjectID = projectID
	return rt
}

func withSandboxCredentials(t *testing.T) {
	t.Helper()
	t.Setenv("SCW_ACCESS_KEY", "SCWTESTACCESSKEY0000")
	t.Setenv("SCW_SECRET_KEY", "11111111-1111-1111-1111-111111111111")
	t.Setenv("SCW_DEFAULT_ORGANIZATION_ID", "22222222-2222-2222-2222-222222222222")
}

// TestSandboxEnvNeverInheritsMockwayURL is the load-bearing regression
// test for this slice.
//
// Layer 3 was code-complete for months while carrying a hole that made
// every green result meaningless: sandboxCommandEnv returns an override
// map, execCommandRunner merges it over os.Environ(), and an override
// map can set but never unset. A developer with SCW_API_URL exported --
// which is exactly what mockway's `make demo-env` writes, and what
// cloudEnv sets for Layer 2 -- got a "real Scaleway" apply that quietly
// went to the mock and reported sandbox_deploy/apply: pass.
//
// This drives a real subprocess rather than inspecting the Command
// struct, because the struct carrying the right StripEnv list proves
// nothing if the runner ignores it.
func TestSandboxEnvNeverInheritsMockwayURL(t *testing.T) {
	t.Setenv("SCW_API_URL", "http://localhost:8080")
	t.Setenv("SCW_PROFILE", "myProfile")
	t.Setenv("SCW_DEFAULT_PROJECT_ID", "00000000-0000-0000-0000-000000000000")

	result, err := execCommandRunner{}.Run(context.Background(), harness.Command{
		Name: "sh",
		Args: []string{"-c", `printf 'api=%s profile=%s project=%s' "${SCW_API_URL-UNSET}" "${SCW_PROFILE-UNSET}" "${SCW_DEFAULT_PROJECT_ID-UNSET}"`},
		Env: map[string]string{
			"SCW_ACCESS_KEY": "SCWTESTACCESSKEY0000",
		},
		StripEnv: harness.SandboxStripEnv,
	})
	if err != nil {
		t.Fatalf("run probe subprocess: %v", err)
	}

	got := string(result.Stdout)
	want := "api=UNSET profile=UNSET project=UNSET"
	if got != want {
		t.Fatalf("sandbox subprocess inherited a mock-targeting environment\nwant: %q\ngot:  %q", want, got)
	}
}

// TestSandboxHarnessesDeclareStripEnv guards the wiring: stripEnvKeys
// only helps if every sandbox command actually asks for it. A new stage
// added to SandboxDeployHarness.Run without StripEnv would reopen the
// hole for that one command.
func TestSandboxHarnessesDeclareStripEnv(t *testing.T) {
	t.Parallel()

	if !slices.Contains(harness.SandboxStripEnv, "SCW_API_URL") {
		t.Fatalf("SandboxStripEnv must strip SCW_API_URL, got %v", harness.SandboxStripEnv)
	}

	recorder := &recordingRunner{}
	if _, err := harness.NewSandboxDeployHarness(recorder).Run(context.Background(), t.TempDir(), map[string]string{}); err != nil {
		t.Fatalf("sandbox deploy: %v", err)
	}
	if _, err := harness.NewSandboxDestroyHarness(recorder).Run(context.Background(), t.TempDir(), map[string]string{}); err != nil {
		t.Fatalf("sandbox destroy: %v", err)
	}

	if len(recorder.commands) != 4 {
		t.Fatalf("expected 4 sandbox commands (init, plan, apply, destroy), got %d", len(recorder.commands))
	}
	for _, cmd := range recorder.commands {
		if !slices.Contains(cmd.StripEnv, "SCW_API_URL") {
			t.Fatalf("sandbox command %v does not strip SCW_API_URL (StripEnv=%v)", cmd.Args, cmd.StripEnv)
		}
	}
}

type recordingRunner struct {
	commands []harness.Command
}

func (r *recordingRunner) Run(_ context.Context, cmd harness.Command) (harness.CommandResult, error) {
	r.commands = append(r.commands, cmd)
	return harness.CommandResult{}, nil
}

func TestSandboxPreflightRejectsNonScalewayEndpoint(t *testing.T) {
	isolateSCWConfig(t)
	withSandboxCredentials(t)
	t.Setenv("SCW_API_URL", "http://localhost:8080")

	_, err := sandboxCommandEnv(sandboxTestRuntime())
	if err == nil {
		t.Fatal("expected sandbox preflight to refuse an inherited mockway endpoint, got nil error")
	}
	if !strings.Contains(err.Error(), "http://localhost:8080") {
		t.Fatalf("error should name the offending endpoint so the operator can see it, got: %v", err)
	}
}

func TestSandboxPreflightAcceptsRealScalewayEndpoint(t *testing.T) {
	isolateSCWConfig(t)
	withSandboxCredentials(t)
	t.Setenv("SCW_API_URL", realScalewayAPIURL)

	if _, err := sandboxCommandEnv(sandboxTestRuntime()); err != nil {
		t.Fatalf("explicit real endpoint should be accepted, got: %v", err)
	}
}

// TestSandboxPreflightRejectsConfigFileEndpoint covers the subtle half
// of the hole. Stripping env vars is not sufficient: the Scaleway SDK
// reads ~/.config/scw/config.yaml regardless, and its top-level keys are
// the default profile.
func TestSandboxPreflightRejectsConfigFileEndpoint(t *testing.T) {
	withSandboxCredentials(t)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "scw"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scw", "config.yaml"),
		[]byte("access_key: SCWXXX\napi_url: http://127.0.0.1:8080\n"), 0o600); err != nil {
		t.Fatalf("write scw config: %v", err)
	}

	_, err := sandboxCommandEnv(sandboxTestRuntime())
	if err == nil {
		t.Fatal("expected refusal when the default scw profile redirects api_url")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:8080") {
		t.Fatalf("error should name the config-file endpoint, got: %v", err)
	}
}

// A `profiles:` entry must NOT trip the assertion: SCW_PROFILE is
// stripped, so a named profile is unreachable and its api_url is
// irrelevant. This machine's real config has exactly this shape.
func TestSandboxPreflightIgnoresNamedProfileEndpoint(t *testing.T) {
	withSandboxCredentials(t)
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "scw"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scw", "config.yaml"),
		[]byte("access_key: SCWXXX\nprofiles:\n  myProfile:\n    api_url: http://127.0.0.1:8080\n"), 0o600); err != nil {
		t.Fatalf("write scw config: %v", err)
	}

	if _, err := sandboxCommandEnv(sandboxTestRuntime()); err != nil {
		t.Fatalf("a named profile is unreachable once SCW_PROFILE is stripped; should not fail preflight: %v", err)
	}
}

func TestSandboxCommandEnvRequiresOrganizationID(t *testing.T) {
	isolateSCWConfig(t)
	t.Setenv("SCW_ACCESS_KEY", "SCWTESTACCESSKEY0000")
	t.Setenv("SCW_SECRET_KEY", "11111111-1111-1111-1111-111111111111")
	t.Setenv("SCW_DEFAULT_ORGANIZATION_ID", "")

	_, err := sandboxCommandEnv(sandboxTestRuntime())
	if err == nil {
		t.Fatal("expected sandbox preflight to require SCW_DEFAULT_ORGANIZATION_ID")
	}
	if !strings.Contains(err.Error(), "SCW_DEFAULT_ORGANIZATION_ID") {
		t.Fatalf("error should name the missing variable, got: %v", err)
	}
}

func TestSandboxCommandEnvCarriesPlacementAndOmitsAPIURL(t *testing.T) {
	isolateSCWConfig(t)
	withSandboxCredentials(t)

	env, err := sandboxCommandEnv(sandboxTestRuntime())
	if err != nil {
		t.Fatalf("sandboxCommandEnv: %v", err)
	}
	if _, ok := env["SCW_API_URL"]; ok {
		t.Fatalf("sandbox env must not set SCW_API_URL at all; the provider default is the real endpoint. got %v", env)
	}
	for key, want := range map[string]string{
		"SCW_DEFAULT_ORGANIZATION_ID": "22222222-2222-2222-2222-222222222222",
		"SCW_DEFAULT_REGION":          "fr-par",
		"SCW_DEFAULT_ZONE":            "fr-par-1",
	} {
		if env[key] != want {
			t.Fatalf("env[%s] = %q, want %q", key, env[key], want)
		}
	}
}

// sandboxCredsForTest supplies the full Layer 3 credential set and
// isolates the scw config lookup. Both matter: sandboxCommandEnv now
// requires an organization id (scaleway_account_project has to be
// created somewhere), and it refuses to run when the default scw
// profile redirects api_url -- which would otherwise make these tests
// pass or fail depending on the developer's ~/.config/scw/config.yaml.
func sandboxCredsForTest(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SCW_ACCESS_KEY", "real-access")
	t.Setenv("SCW_SECRET_KEY", "real-secret")
	t.Setenv("SCW_DEFAULT_ORGANIZATION_ID", "22222222-2222-2222-2222-222222222222")
}

// A resource whose HCL omits project_id does not fail -- it lands in
// whatever the provider resolves as the default project, which on a
// normal account is the organization default, next to real
// infrastructure. Pointing the default at a dedicated throwaway project
// contains that. Detection stays with the orphan sweep; this is
// containment.
func TestSandboxEnvPinsFallbackProject(t *testing.T) {
	isolateSCWConfig(t)
	withSandboxCredentials(t)

	env, err := sandboxCommandEnv(sandboxTestRuntimeWithFallback("2397e80e-ec12-4a7e-819f-a2caba3867b6"))
	if err != nil {
		t.Fatalf("sandboxCommandEnv: %v", err)
	}
	if env["SCW_DEFAULT_PROJECT_ID"] != "2397e80e-ec12-4a7e-819f-a2caba3867b6" {
		t.Fatalf("fallback project not pinned, got %q", env["SCW_DEFAULT_PROJECT_ID"])
	}
}

func TestSandboxEnvRejectsOrgAsFallbackProject(t *testing.T) {
	isolateSCWConfig(t)
	withSandboxCredentials(t)

	// The org id IS the default project's id. Accepting it would make the
	// setting a no-op while looking configured.
	_, err := sandboxCommandEnv(sandboxTestRuntimeWithFallback("22222222-2222-2222-2222-222222222222"))
	if err == nil {
		t.Fatal("the organization default project must be refused as a fallback target")
	}
	if !strings.Contains(err.Error(), "fallback_project_id") {
		t.Fatalf("error should name the setting, got: %v", err)
	}
}

func TestSandboxEnvOmitsProjectWhenNoFallbackConfigured(t *testing.T) {
	isolateSCWConfig(t)
	withSandboxCredentials(t)

	env, err := sandboxCommandEnv(sandboxTestRuntime())
	if err != nil {
		t.Fatalf("sandboxCommandEnv: %v", err)
	}
	if _, ok := env["SCW_DEFAULT_PROJECT_ID"]; ok {
		t.Fatal("with no fallback configured the provider default must be left alone")
	}
}
