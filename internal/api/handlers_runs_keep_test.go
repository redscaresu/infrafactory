package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
)

func keepServer(t *testing.T, starter RunStarter, deployer DeploymentDeployer) *httptest.Server {
	t.Helper()
	scenariosDir := filepath.Join(t.TempDir(), "scenarios")
	require.NoError(t, os.MkdirAll(filepath.Join(scenariosDir, "training"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scenariosDir, "training", "web.yaml"),
		[]byte(validScenarioYAML("web-app-paris", "test")), 0o644))

	cfg := config.Default()
	cfg.Paths.Scenarios = scenariosDir
	srv := NewServer(ServerConfig{Config: cfg, RunStarter: starter, Deployer: deployer})
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)
	return ts
}

func postStart(t *testing.T, ts *httptest.Server, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/runs/web-app-paris/start",
		bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// The disabled checkbox is a hint; this is the guard. Leaving a stack
// running is the same grant as deploying one (ADR-0027), so a server
// started with --allow-layer3 alone must refuse -- what makes that
// server the safer one is that every run destroys what it made.
func TestRunsStartRefusesKeepWithoutTheDeployGrant(t *testing.T) {
	t.Parallel()

	starter := &fakeStarter{}
	ts := keepServer(t, starter, nil)

	resp := postStart(t, ts, `{"keep":true}`)

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body["error"], "--allow-deploy")
	assert.Equal(t, StartRunRequest{}, starter.lastReq, "the run must not have started")
}

func TestRunsStartCarriesKeepWhenDeployIsGranted(t *testing.T) {
	t.Parallel()

	starter := &fakeStarter{}
	ts := keepServer(t, starter, &fakeDeployer{})

	resp := postStart(t, ts, `{"keep":true}`)

	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.True(t, starter.lastReq.Keep, "the flag has to reach the run or the checkbox does nothing")
}

// Both skip a destroy and mean opposite things about what happens
// next, so accepting both would leave it ambiguous which was wanted.
func TestRunsStartRejectsKeepWithNoDestroy(t *testing.T) {
	t.Parallel()

	starter := &fakeStarter{}
	ts := keepServer(t, starter, &fakeDeployer{})

	resp := postStart(t, ts, `{"keep":true,"no_destroy":true}`)

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	assert.Equal(t, StartRunRequest{}, starter.lastReq, "the run must not have started")
}

// The run page reads this to decide whether the box is tickable, and
// reports it separately from layer3 because they are separate grants.
func TestLayer3StatusReportsTheKeepGrantSeparately(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		deployer DeploymentDeployer
		want     bool
	}{
		{name: "no deploy grant", deployer: nil, want: false},
		{name: "deploy granted", deployer: &fakeDeployer{}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts := keepServer(t, &fakeStarter{}, tc.deployer)

			resp, err := http.Get(ts.URL + "/api/scenarios/training/web/layer3-status")
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			var body map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			assert.Equal(t, tc.want, body["server_allows_keep"])
		})
	}
}
