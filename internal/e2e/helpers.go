// Package e2e provides cross-repo end-to-end test infrastructure for
// infrafactory + mockway integration tests. Tests in this package start a
// real mockway binary from the sibling `../mockway` source repo and drive
// the infrafactory CLI in-process with stub generators that return
// pre-baked HCL.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/redscaresu/infrafactory/internal/cli"
	"github.com/redscaresu/infrafactory/internal/generator"
)

const (
	// EnvEnableE2E gates execution of e2e tests in this package. The tests
	// shell out to `go run ./cmd/mockway` and (in S33-T2/T3) `tofu`, which
	// makes them too heavy for the default unit-test path.
	EnvEnableE2E = "INFRAFACTORY_ENABLE_E2E"

	mockwayReadinessTimeout = 30 * time.Second
)

// SkipUnlessEnabled skips the test unless the e2e env gate is set.
func SkipUnlessEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv(EnvEnableE2E) != "1" {
		t.Skipf("set %s=1 to enable e2e tests", EnvEnableE2E)
	}
}

// MockwayInstance describes a running mockway process started by tests.
type MockwayInstance struct {
	URL      string
	cmd      *exec.Cmd
	logFile  *os.File
	logPath  string
	stopOnce sync.Once
}

// Stop kills the mockway process and releases its log file. Safe to call
// multiple times.
func (m *MockwayInstance) Stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		if m.cmd != nil && m.cmd.Process != nil {
			_ = m.cmd.Process.Kill()
			_ = m.cmd.Wait()
		}
		if m.logFile != nil {
			_ = m.logFile.Close()
		}
	})
}

// LogPath returns the file path that mockway's stdout+stderr is written to.
// Useful for diagnostics on test failures.
func (m *MockwayInstance) LogPath() string {
	if m == nil {
		return ""
	}
	return m.logPath
}

// Reset clears mockway state via POST /mock/reset.
func (m *MockwayInstance) Reset(t *testing.T) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, m.URL+"/mock/reset", nil)
	if err != nil {
		t.Fatalf("build mockway reset request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("mockway reset: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		t.Fatalf("mockway reset: unexpected status %d", resp.StatusCode)
	}
}

// FetchState returns mockway's current state.
func (m *MockwayInstance) FetchState(t *testing.T) map[string]any {
	t.Helper()
	resp, err := http.Get(m.URL + "/mock/state")
	if err != nil {
		t.Fatalf("get mock state: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get mock state: unexpected status %d", resp.StatusCode)
	}
	var state map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("decode mock state: %v", err)
	}
	return state
}

// StartMockway compiles and starts mockway from the sibling `../mockway`
// source repo on a free local port. It registers a t.Cleanup hook to kill
// the process when the test ends.
func StartMockway(t *testing.T) *MockwayInstance {
	t.Helper()
	return startSiblingMock(t, "mockway", "../mockway", "./cmd/mockway")
}

// StartFakegcp compiles and starts fakegcp from the sibling
// `../fakegcp` source repo on a free local port. Mirror of
// StartMockway — fakegcp exposes the same `/mock/{state,reset,
// snapshot,restore}` admin surface, so the returned MockwayInstance
// helpers (FetchState/Reset/Stop) work against it without changes.
func StartFakegcp(t *testing.T) *MockwayInstance {
	t.Helper()
	return startSiblingMock(t, "fakegcp", "../fakegcp", "./cmd/fakegcp")
}

// StartFakeaws compiles and starts fakeaws from the sibling
// `../fakeaws` source repo on a free local port. Mirror of
// StartMockway / StartFakegcp — fakeaws exposes the same
// `/mock/{state,reset,snapshot,restore}` admin surface (per
// fakeaws/handlers/admin.go § stateSchemaVersion), so the returned
// MockwayInstance helpers (FetchState/Reset/Stop) work against it
// without changes.
//
// Per fakeaws/concepts.md "Required surface" item 10 (S43-T9):
// signature mirrors StartFakegcp so test code is uniform across
// clouds. The returned URL goes into Config.Fakeaws.URL via the
// helper's config-writer (extended in S43-T9 to include the
// fakeaws block + 'aws' in policy_paths).
func StartFakeaws(t *testing.T) *MockwayInstance {
	t.Helper()
	return startSiblingMock(t, "fakeaws", "../fakeaws", "./cmd/fakeaws")
}

func startSiblingMock(t *testing.T, name, repoRel, cmdPath string) *MockwayInstance {
	t.Helper()

	port := pickFreePort(t)
	dbPath := filepath.Join(t.TempDir(), name+".sqlite")
	logPath := filepath.Join(t.TempDir(), name+".log")
	repoRoot := resolveSiblingRepo(t, repoRel)

	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create %s log file: %v", name, err)
	}

	cmd := exec.Command("go", "run", cmdPath, "--port", fmt.Sprintf("%d", port), "--db", dbPath)
	cmd.Dir = repoRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start %s: %v", name, err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitForHTTP200(url+"/mock/state", mockwayReadinessTimeout); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = logFile.Close()
		logPayload, _ := os.ReadFile(logPath)
		t.Fatalf("wait for %s readiness: %v\nlog:\n%s", name, err, string(logPayload))
	}

	instance := &MockwayInstance{
		URL:     url,
		cmd:     cmd,
		logFile: logFile,
		logPath: logPath,
	}
	t.Cleanup(instance.Stop)
	return instance
}

// resolveSiblingRepo returns the absolute path to a sibling source
// repo (e.g. ../mockway, ../fakegcp). Fails the test if the repo's
// command package isn't found.
func resolveSiblingRepo(t *testing.T, rel string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", rel))
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("resolve sibling %s repo at %s: %v", rel, root, err)
	}
	return root
}

func waitForHTTP200(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(250 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("timeout waiting for %s: %w", url, lastErr)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

func pickFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// InfrafactoryRunOptions configures a single in-process invocation of an
// infrafactory CLI command driven by a stub generator.
type InfrafactoryRunOptions struct {
	// Args are the command-line arguments passed to the cobra command,
	// for example []string{"run", "scenario.yaml", "--config", "config.yaml"}.
	// The leading binary name is omitted.
	Args []string

	// GeneratorFiles is the map returned by the stub generator for every
	// generation request. Use this when every iteration should produce the
	// same HCL.
	GeneratorFiles map[string][]byte

	// GeneratorFunc, if non-nil, replaces GeneratorFiles. Useful when
	// different iterations should produce different HCL (e.g. incremental
	// stages keyed off the scenario YAML).
	GeneratorFunc func(ctx context.Context, req generator.Request) (*generator.GeneratedCode, error)
}

// InfrafactoryResult captures the stdout/stderr of an in-process CLI
// invocation along with the command error (if any).
type InfrafactoryResult struct {
	Stdout string
	Stderr string
	Err    error
}

// RunInfrafactory drives the infrafactory CLI in-process with a stub
// generator. It is the cross-repo equivalent of `go run ./cmd/infrafactory
// <args>` but avoids subprocess + recompile overhead.
func RunInfrafactory(t *testing.T, opts InfrafactoryRunOptions) InfrafactoryResult {
	t.Helper()

	gen := opts.GeneratorFunc
	if gen == nil {
		files := opts.GeneratorFiles
		gen = func(_ context.Context, _ generator.Request) (*generator.GeneratedCode, error) {
			return &generator.GeneratedCode{Files: cloneFiles(files)}, nil
		}
	}

	deps := cli.RuntimeDependencies{
		Generator: generator.SeedGeneratorFunc(gen),
	}

	root := cli.NewRootCmd(cli.WithRuntimeDependencies(deps))
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(opts.Args)

	err := root.Execute()
	return InfrafactoryResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
		Err:    err,
	}
}

func cloneFiles(src map[string][]byte) map[string][]byte {
	if src == nil {
		return nil
	}
	out := make(map[string][]byte, len(src))
	for name, content := range src {
		copied := make([]byte, len(content))
		copy(copied, content)
		out[name] = copied
	}
	return out
}

// WriteFile creates parent directories as needed and writes content to path.
// Convenience helper for staging scenario YAMLs and config files in test
// workspaces.
func WriteFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// WriteConfig writes a typical infrafactory.yaml that targets the given
// mockway URL and output root with all validation layers configured for
// hermetic mockway-only runs (sandbox deploy disabled). Policy, mapping,
// prompt, and pitfall paths are resolved against the repository root so
// scenarios with policy criteria can be evaluated end-to-end.
//
// fakegcp + fakeaws URLs are left empty; for GCP/AWS scenarios use
// WriteConfigMultiCloud so the cloudMockStateRouter has a target for
// the matching cloud.
func WriteConfig(t *testing.T, configPath, mockwayURL, outputRoot string) {
	t.Helper()
	WriteConfigMultiCloud(t, configPath, mockwayURL, "", "", outputRoot)
}

// WriteConfigMultiCloud writes an infrafactory.yaml with mockway,
// fakegcp, and fakeaws URLs populated. Any URL may be empty when the
// matching cloud isn't exercised by the test; pass a non-empty URL
// when the test runs a `cloud: gcp` or `cloud: aws` scenario through
// the cloudMockStateRouter.
func WriteConfigMultiCloud(t *testing.T, configPath, mockwayURL, fakegcpURL, fakeawsURL, outputRoot string) {
	t.Helper()
	repoRoot := RepoRoot(t)
	WriteFile(t, configPath, fmt.Appendf(nil, `version: "1.0"
agent:
  type: claude-code
mockway:
  url: %s
fakegcp:
  url: %s
fakeaws:
  url: %s
scaleway:
  credentials_source: env
validation:
  layers:
    static:
      enabled: true
      policy_paths: [%s/policies/common, %s/policies/scaleway, %s/policies/gcp, %s/policies/aws]
    mock_deploy:
      enabled: true
    sandbox_deploy:
      enabled: false
    destruction:
      enabled: true
constraint_policies:
  no_public_database: scaleway/no_public_database.rego
  encryption_at_rest: scaleway/encryption_at_rest.rego
  no_public_endpoints: scaleway/no_public_endpoints.rego
  region_restriction: scaleway/region_restriction.rego
  region: scaleway/region_restriction.rego
  zone: scaleway/region_restriction.rego
paths:
  scenarios: %s/scenarios
  mappings: %s/mappings.yaml
  output: %s
  policies: %s/policies
  prompts: %s/prompts
  pitfalls: %s/pitfalls
`,
		mockwayURL,
		fakegcpURL,
		fakeawsURL,
		repoRoot, repoRoot, repoRoot, repoRoot,
		repoRoot, repoRoot, outputRoot, repoRoot, repoRoot, repoRoot,
	))
}

// RepoRoot returns the absolute path to the infrafactory repository root,
// resolved relative to this source file. Useful for tests that need to
// reference repo-checked-in fixtures (scenarios, policies, mappings).
func RepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}
