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

	port := pickFreePort(t)
	dbPath := filepath.Join(t.TempDir(), "mockway.sqlite")
	logPath := filepath.Join(t.TempDir(), "mockway.log")
	mockwayRoot := resolveSiblingMockwayRoot(t)

	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create mockway log file: %v", err)
	}

	cmd := exec.Command("go", "run", "./cmd/mockway", "--port", fmt.Sprintf("%d", port), "--db", dbPath)
	cmd.Dir = mockwayRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start mockway: %v", err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitForHTTP200(url+"/mock/state", mockwayReadinessTimeout); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = logFile.Close()
		logPayload, _ := os.ReadFile(logPath)
		t.Fatalf("wait for mockway readiness: %v\nlog:\n%s", err, string(logPayload))
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

// resolveSiblingMockwayRoot returns the absolute path to the sibling
// `../mockway` source repo. Fails the test if the repo is missing.
func resolveSiblingMockwayRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "mockway"))
	if _, err := os.Stat(filepath.Join(root, "cmd", "mockway")); err != nil {
		t.Fatalf("resolve sibling mockway repo at %s: %v", root, err)
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
// hermetic mockway-only runs (sandbox deploy disabled).
func WriteConfig(t *testing.T, configPath, mockwayURL, outputRoot string) {
	t.Helper()
	WriteFile(t, configPath, fmt.Appendf(nil, `version: "1.0"
agent:
  type: claude-code
mockway:
  url: %s
paths:
  output: %s
validation:
  layers:
    static:
      enabled: true
      policy_paths: []
    mock_deploy:
      enabled: true
    sandbox_deploy:
      enabled: false
    destruction:
      enabled: true
`, mockwayURL, outputRoot))
}
