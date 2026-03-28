package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/runstore"
)

func TestScenariosHandlersListAndGet(t *testing.T) {
	t.Parallel()

	scenariosDir := filepath.Join(t.TempDir(), "scenarios")
	mustWriteScenarioFile(t, filepath.Join(scenariosDir, "training", "web-app-paris.yaml"), validScenarioYAML("web-app-paris", "training web app"))
	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	_ = store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:       "web-app-paris",
		RunID:          "run-1",
		Status:         "success",
		TerminalReason: "target_reached",
		StartedAt:      time.Now().UTC(),
	})

	cfg := config.Default()
	cfg.Paths.Scenarios = scenariosDir
	srv := NewServer(ServerConfig{Config: cfg, Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/scenarios")
	if err != nil {
		t.Fatalf("get scenarios list: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var listPayload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&listPayload); err != nil {
		t.Fatalf("decode list payload: %v", err)
	}
	if _, ok := listPayload["groups"]; !ok {
		t.Fatalf("expected groups in list payload")
	}

	detailResp, err := http.Get(ts.URL + "/api/scenarios/training/web-app-paris")
	if err != nil {
		t.Fatalf("get scenario detail: %v", err)
	}
	defer detailResp.Body.Close()
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", detailResp.StatusCode)
	}
	var detail map[string]any
	if err := json.NewDecoder(detailResp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail payload: %v", err)
	}
	if got := detail["name"]; got != "web-app-paris" {
		t.Fatalf("expected scenario name web-app-paris, got %#v", got)
	}
}

func TestScenariosPutValidationAndTraversal(t *testing.T) {
	t.Parallel()

	scenariosDir := filepath.Join(t.TempDir(), "scenarios")
	target := filepath.Join(scenariosDir, "training", "web-app-paris.yaml")
	mustWriteScenarioFile(t, target, validScenarioYAML("web-app-paris", "initial"))

	cfg := config.Default()
	cfg.Paths.Scenarios = scenariosDir
	srv := NewServer(ServerConfig{Config: cfg})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/scenarios/training/web-app-paris", bytes.NewBufferString(validScenarioYAML("web-app-paris", "updated")))
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put valid scenario: %v", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", putResp.StatusCode)
	}

	reqInvalid, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/scenarios/training/web-app-paris", bytes.NewBufferString("scenario: only-name\n"))
	invalidResp, err := http.DefaultClient.Do(reqInvalid)
	if err != nil {
		t.Fatalf("put invalid scenario: %v", err)
	}
	defer invalidResp.Body.Close()
	if invalidResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", invalidResp.StatusCode)
	}

	reqMalformed, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/scenarios/training/web-app-paris", bytes.NewBufferString("scenario: [\n"))
	malformedResp, err := http.DefaultClient.Do(reqMalformed)
	if err != nil {
		t.Fatalf("put malformed scenario: %v", err)
	}
	defer malformedResp.Body.Close()
	if malformedResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", malformedResp.StatusCode)
	}

	traversalReq := httptest.NewRequest(http.MethodPut, "/api/scenarios/../../etc/passwd", bytes.NewBufferString(validScenarioYAML("web-app-paris", "updated")))
	traversalRec := httptest.NewRecorder()
	scenarioByPathHandler(&serverState{cfg: cfg}).ServeHTTP(traversalRec, traversalReq)
	if traversalRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", traversalRec.Code)
	}

	written, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target scenario: %v", err)
	}
	if !strings.Contains(string(written), "updated") {
		t.Fatalf("expected updated scenario content, got:\n%s", string(written))
	}
}

func TestScenarioRunModeHandlerDetectsIncrementalAndClean(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	scenariosDir := filepath.Join(root, "scenarios")
	outputDir := filepath.Join(root, "output")
	mustWriteScenarioFile(t, filepath.Join(scenariosDir, "training", "web-app-paris.yaml"), validScenarioYAML("web-app-paris", "training web app"))
	if err := os.MkdirAll(filepath.Join(outputDir, "web-app-paris"), 0o755); err != nil {
		t.Fatalf("mkdir output dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "web-app-paris", "terraform.tfstate"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write tfstate: %v", err)
	}

	store := runstore.NewFilesystemStore(filepath.Join(root, ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:  "web-app-paris",
		RunID:     "run-1",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Scenarios = scenariosDir
	cfg.Paths.Output = outputDir
	srv := NewServer(ServerConfig{
		Config:    cfg,
		Store:     store,
		MockState: staticMockStateReader{payload: []byte(`{"instance":{"servers":[{"id":"srv-1"}]}}`)},
	})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/scenarios/training/web-app-paris/run-mode")
	if err != nil {
		t.Fatalf("get run mode: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["mode"] != "incremental" {
		t.Fatalf("expected incremental mode, got %+v", payload)
	}

	cleanSrv := NewServer(ServerConfig{
		Config:    cfg,
		Store:     runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs")),
		MockState: staticMockStateReader{payload: []byte(`{"instance":{"servers":[]}}`)},
	})
	cleanTS := httptest.NewServer(cleanSrv.Handler)
	defer cleanTS.Close()

	cleanResp, err := http.Get(cleanTS.URL + "/api/scenarios/training/web-app-paris/run-mode")
	if err != nil {
		t.Fatalf("get clean run mode: %v", err)
	}
	defer cleanResp.Body.Close()
	if cleanResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", cleanResp.StatusCode)
	}
	if err := json.NewDecoder(cleanResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode clean payload: %v", err)
	}
	if payload["mode"] != "clean" {
		t.Fatalf("expected clean mode, got %+v", payload)
	}
}

func TestScenarioLayer3StatusHandlerReportsReadiness(t *testing.T) {
	root := t.TempDir()
	scenariosDir := filepath.Join(root, "scenarios")
	mustWriteScenarioFile(t, filepath.Join(scenariosDir, "training", "web-app-paris.yaml"), validScenarioYAML("web-app-paris", "training web app"))

	cfg := config.Default()
	cfg.Paths.Scenarios = scenariosDir
	cfg.Validation.Layers.SandboxDeploy.Enabled = true
	t.Setenv("SCW_ACCESS_KEY", "access")
	t.Setenv("SCW_SECRET_KEY", "secret")

	srv := NewServer(ServerConfig{Config: cfg})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/scenarios/training/web-app-paris/layer3-status")
	if err != nil {
		t.Fatalf("get layer3 status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode layer3 payload: %v", err)
	}
	if payload["ready"] != true || payload["config_default_enabled"] != true {
		t.Fatalf("expected ready/config enabled payload, got %+v", payload)
	}
}

func mustWriteScenarioFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir scenario dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write scenario file: %v", err)
	}
}

func validScenarioYAML(name, description string) string {
	return `scenario: ` + name + `
version: "1.0"
cloud: scaleway
description: ` + description + `
resources:
  compute:
    purpose: web-server
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`
}

type staticMockStateReader struct {
	payload []byte
}

func (s staticMockStateReader) State(context.Context) ([]byte, error) {
	return s.payload, nil
}
