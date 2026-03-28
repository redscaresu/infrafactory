package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/runstore"
)

func TestRunsHandlers(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	now := time.Now().UTC()
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:       "web-app-paris",
		RunID:          "run-1",
		Status:         "success",
		TerminalReason: "target_reached",
		StartedAt:      now,
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}
	if err := store.WriteIterationArtifact("web-app-paris", "run-1", 1, "iteration.json", []byte(`{"iteration":1}`)); err != nil {
		t.Fatalf("write iteration: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	for _, path := range []string{
		"/api/runs",
		"/api/runs/web-app-paris",
		"/api/runs/web-app-paris/run-1",
		"/api/runs/web-app-paris/run-1/iterations/1",
	} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", path, resp.StatusCode)
		}
	}

	notFound, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-missing")
	if err != nil {
		t.Fatalf("get missing run: %v", err)
	}
	notFound.Body.Close()
	if notFound.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", notFound.StatusCode)
	}
}

func TestListAllRunsReturnsNewestFirstAcrossScenarios(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	metas := []runstore.RunMetadata{
		{
			Scenario:  "a-scenario",
			RunID:     "20260228T100000Z",
			Status:    "success",
			StartedAt: time.Now().UTC(),
		},
		{
			Scenario:  "z-scenario",
			RunID:     "20260228T120000Z",
			Status:    "failed",
			StartedAt: time.Now().UTC(),
		},
		{
			Scenario:  "m-scenario",
			RunID:     "20260228T110000Z",
			Status:    "success",
			StartedAt: time.Now().UTC(),
		},
	}
	for _, meta := range metas {
		if err := store.WriteRunMetadata(meta); err != nil {
			t.Fatalf("write run metadata: %v", err)
		}
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs")
	if err != nil {
		t.Fatalf("get /api/runs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Runs []runstore.RunMetadata `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(payload.Runs))
	}
	if payload.Runs[0].RunID != "20260228T120000Z" || payload.Runs[1].RunID != "20260228T110000Z" || payload.Runs[2].RunID != "20260228T100000Z" {
		t.Fatalf("unexpected run order: %+v", payload.Runs)
	}
}

func TestRunsHandlersIgnoreIncompleteRunDirectories(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	now := time.Now().UTC()
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:       "web-app-paris",
		RunID:          "run-1",
		Status:         "success",
		TerminalReason: "target_reached",
		StartedAt:      now,
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(store.Root, "web-app-paris", "run-2"), 0o755); err != nil {
		t.Fatalf("mkdir incomplete run: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs")
	if err != nil {
		t.Fatalf("get /api/runs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRunsHandlersIgnoreHistoricalIncompleteRunDirectoryNames(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	now := time.Now().UTC()
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:       "k8s-cluster-paris",
		RunID:          "20260225T091949Z",
		Status:         "success",
		TerminalReason: "target_reached",
		StartedAt:      now,
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}
	incomplete := filepath.Join(store.Root, "k8s-cluster-paris", "20260225T003146Z")
	if err := os.MkdirAll(incomplete, 0o755); err != nil {
		t.Fatalf("mkdir incomplete historical run: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs")
	if err != nil {
		t.Fatalf("get /api/runs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Runs []runstore.RunMetadata `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Runs) != 1 || payload.Runs[0].RunID != "20260225T091949Z" {
		t.Fatalf("unexpected runs payload: %+v", payload.Runs)
	}
}

func TestRunGeneratedFilesHandlersListAndRead(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:  "web-app-paris",
		RunID:     "run-1",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}
	if err := store.WriteGeneratedFiles("web-app-paris", "run-1", map[string][]byte{
		"main.tf": []byte("terraform {}\n"),
	}); err != nil {
		t.Fatalf("write generated files: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	listResp, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-1/files")
	if err != nil {
		t.Fatalf("get file list: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	fileResp, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-1/files/main.tf")
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	defer fileResp.Body.Close()
	if fileResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", fileResp.StatusCode)
	}
}

func TestRunGeneratedFilesHandlersFormatOnRead(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:  "web-app-paris",
		RunID:     "run-1",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}
	if err := store.WriteGeneratedFiles("web-app-paris", "run-1", map[string][]byte{
		"main.tf": []byte("resource \"x\" \"y\"{}"),
	}); err != nil {
		t.Fatalf("write generated files: %v", err)
	}

	formatter := &fakeFormatter{formatted: []byte("resource \"x\" \"y\" {}\n")}
	srv := NewServer(ServerConfig{Config: config.Default(), Store: store, Formatter: formatter})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	fileResp, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-1/files/main.tf?format=1")
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	body, _ := io.ReadAll(fileResp.Body)
	fileResp.Body.Close()
	if fileResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", fileResp.StatusCode)
	}
	if string(body) != string(formatter.formatted) {
		t.Fatalf("expected formatted output, got %q", string(body))
	}
}

func TestRunIterationHandlersListReadAndFiles(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:  "web-app-paris",
		RunID:     "run-1",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}
	if err := store.WriteIterationArtifact("web-app-paris", "run-1", 1, "iteration.json", []byte(`{"iteration":1}`)); err != nil {
		t.Fatalf("write iteration artifact: %v", err)
	}
	if err := store.WriteIterationGeneratedFiles("web-app-paris", "run-1", 1, map[string][]byte{
		"main.tf": []byte("resource \"scaleway_vpc\" \"iter1\" {}\n"),
	}); err != nil {
		t.Fatalf("write iteration generated files: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-1/iterations")
	if err != nil {
		t.Fatalf("get iterations: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var list struct {
		Iterations []int `json:"iterations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode iterations: %v", err)
	}
	if len(list.Iterations) != 1 || list.Iterations[0] != 1 {
		t.Fatalf("unexpected iterations: %+v", list.Iterations)
	}

	fileResp, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-1/iterations/1/files/main.tf")
	if err != nil {
		t.Fatalf("get iteration file: %v", err)
	}
	defer fileResp.Body.Close()
	if fileResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", fileResp.StatusCode)
	}
}

func TestRunGeneratedFilesHandlersRejectTraversal(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-1/files/%2e%2e/%2e%2e/etc/passwd")
	if err != nil {
		t.Fatalf("get traversal path: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestRunBundleHandlerReturnsZipWithFinalAndIterationFiles(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:  "web-app-paris",
		RunID:     "run-1",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}
	if err := store.WriteGeneratedFiles("web-app-paris", "run-1", map[string][]byte{
		"main.tf": []byte("resource \"scaleway_vpc\" \"final\" {}\n"),
	}); err != nil {
		t.Fatalf("write final generated files: %v", err)
	}
	if err := store.WriteIterationGeneratedFiles("web-app-paris", "run-1", 1, map[string][]byte{
		"main.tf": []byte("resource \"scaleway_vpc\" \"iter1\" {}\n"),
	}); err != nil {
		t.Fatalf("write iteration generated files: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-1/bundle.zip")
	if err != nil {
		t.Fatalf("get bundle: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/zip" {
		t.Fatalf("expected zip content type, got %q", got)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	names := make(map[string]bool)
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["generated/main.tf"] {
		t.Fatalf("expected final generated file in bundle, got %+v", names)
	}
	if !names["iterations/1/generated/main.tf"] {
		t.Fatalf("expected iteration generated file in bundle, got %+v", names)
	}
}

func TestRunArtifactsArchiveReturnsWholeRunDirectory(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:       "web-app-paris",
		RunID:          "run-1",
		Status:         "success",
		TerminalReason: "target_reached",
		StartedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}
	if err := store.WriteIterationArtifact("web-app-paris", "run-1", 1, "iteration.json", []byte(`{"iteration":1}`)); err != nil {
		t.Fatalf("write iteration artifact: %v", err)
	}
	if err := store.WriteGeneratedFiles("web-app-paris", "run-1", map[string][]byte{
		"main.tf": []byte("resource \"scaleway_vpc\" \"final\" {}\n"),
	}); err != nil {
		t.Fatalf("write generated files: %v", err)
	}
	if err := store.WriteIterationGeneratedFiles("web-app-paris", "run-1", 1, map[string][]byte{
		"main.tf": []byte("resource \"scaleway_vpc\" \"iter1\" {}\n"),
	}); err != nil {
		t.Fatalf("write iteration generated files: %v", err)
	}
	appLogPath := filepath.Join(store.Root, "web-app-paris", "run-1", "app.log")
	if err := os.WriteFile(appLogPath, []byte(`{"event":"terminal_reason"}`), 0o644); err != nil {
		t.Fatalf("write app log: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-1/artifacts.zip")
	if err != nil {
		t.Fatalf("get artifacts archive: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	names := make(map[string]bool)
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, expected := range []string{"run.json", "app.log", "generated/main.tf", "iterations/1/iteration.json", "iterations/1/generated/main.tf"} {
		if !names[expected] {
			t.Fatalf("expected %s in archive, got %+v", expected, names)
		}
	}
}

func TestRunLogHandlerReturnsAppLog(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:  "web-app-paris",
		RunID:     "run-1",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}
	appLogPath := filepath.Join(store.Root, "web-app-paris", "run-1", "app.log")
	if err := os.WriteFile(appLogPath, []byte("{\"event\":\"run_start\"}\n"), 0o644); err != nil {
		t.Fatalf("write app log: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-1/log")
	if err != nil {
		t.Fatalf("get run log: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "{\"event\":\"run_start\"}\n" {
		t.Fatalf("unexpected app log payload: %q", string(body))
	}
}

func TestRunPlanAndBaselineHandlersReturnArtifacts(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:  "web-app-paris",
		RunID:     "run-1",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}
	if err := store.WriteRunArtifact("web-app-paris", "run-1", "plan.txt", []byte("Plan: 1 to add, 0 to change, 0 to destroy.\n")); err != nil {
		t.Fatalf("write plan artifact: %v", err)
	}
	if err := store.WriteRunArtifact("web-app-paris", "run-1", "baseline_state.json", []byte(`{"instance":{"servers":[{"id":"srv-1"}]}}`)); err != nil {
		t.Fatalf("write baseline artifact: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	planResp, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-1/plan")
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	defer planResp.Body.Close()
	if planResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for plan, got %d", planResp.StatusCode)
	}
	planBody, _ := io.ReadAll(planResp.Body)
	if string(planBody) != "Plan: 1 to add, 0 to change, 0 to destroy.\n" {
		t.Fatalf("unexpected plan payload: %q", string(planBody))
	}

	baselineResp, err := http.Get(ts.URL + "/api/runs/web-app-paris/run-1/baseline")
	if err != nil {
		t.Fatalf("get baseline: %v", err)
	}
	defer baselineResp.Body.Close()
	if baselineResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for baseline, got %d", baselineResp.StatusCode)
	}
	baselineBody, _ := io.ReadAll(baselineResp.Body)
	if string(baselineBody) != `{"instance":{"servers":[{"id":"srv-1"}]}}` {
		t.Fatalf("unexpected baseline payload: %s", string(baselineBody))
	}
}

func TestRunPlanAndBaselineHandlersReturn404WhenMissing(t *testing.T) {
	t.Parallel()

	store := runstore.NewFilesystemStore(filepath.Join(t.TempDir(), ".infrafactory", "runs"))
	if err := store.WriteRunMetadata(runstore.RunMetadata{
		Scenario:  "web-app-paris",
		RunID:     "run-1",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write run metadata: %v", err)
	}

	srv := NewServer(ServerConfig{Config: config.Default(), Store: store})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	for _, path := range []string{
		"/api/runs/web-app-paris/run-1/plan",
		"/api/runs/web-app-paris/run-1/baseline",
	} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for %s, got %d", path, resp.StatusCode)
		}
	}
}

func TestRunsStartReturnsNotImplementedWithoutStarter(t *testing.T) {
	t.Parallel()

	scenariosDir := filepath.Join(t.TempDir(), "scenarios")
	if err := os.MkdirAll(filepath.Join(scenariosDir, "training"), 0o755); err != nil {
		t.Fatalf("mkdir scenarios: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenariosDir, "training", "web.yaml"), []byte(validScenarioYAML("web-app-paris", "test")), 0o644); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Scenarios = scenariosDir
	srv := NewServer(ServerConfig{Config: cfg})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/runs/web-app-paris/start", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post start: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", resp.StatusCode)
	}
}

func TestRunsStartReturnsAcceptedAndConflict(t *testing.T) {
	t.Parallel()

	scenariosDir := filepath.Join(t.TempDir(), "scenarios")
	if err := os.MkdirAll(filepath.Join(scenariosDir, "training"), 0o755); err != nil {
		t.Fatalf("mkdir scenarios: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenariosDir, "training", "web.yaml"), []byte(validScenarioYAML("web-app-paris", "test")), 0o644); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Scenarios = scenariosDir
	starter := &fakeStarter{}
	srv := NewServer(ServerConfig{Config: cfg, RunStarter: starter})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	req1, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/runs/web-app-paris/start", nil)
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("post start 1: %v", err)
	}
	resp1.Body.Close()
	if resp1.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp1.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/runs/web-app-paris/start", nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("post start 2: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp2.StatusCode)
	}
}

func TestRunsStartPassesCleanAndNoDestroyFlags(t *testing.T) {
	t.Parallel()

	scenariosDir := filepath.Join(t.TempDir(), "scenarios")
	if err := os.MkdirAll(filepath.Join(scenariosDir, "training"), 0o755); err != nil {
		t.Fatalf("mkdir scenarios: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenariosDir, "training", "web.yaml"), []byte(validScenarioYAML("web-app-paris", "test")), 0o644); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Scenarios = scenariosDir
	starter := &fakeStarter{}
	srv := NewServer(ServerConfig{Config: cfg, RunStarter: starter})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/runs/web-app-paris/start", bytes.NewBufferString(`{"clean":true,"layer3_enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post start: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	if !starter.lastReq.Clean || starter.lastReq.NoDestroy {
		t.Fatalf("unexpected start request: %+v", starter.lastReq)
	}
	if starter.lastReq.Layer3Enabled == nil || !*starter.lastReq.Layer3Enabled {
		t.Fatalf("expected layer3_enabled to be forwarded, got %+v", starter.lastReq)
	}
	if starter.lastReq.ScenarioName != "web-app-paris" || starter.lastReq.ScenarioPath != "training/web" {
		t.Fatalf("unexpected scenario mapping: %+v", starter.lastReq)
	}
}

func TestRunsStartRejectsMutuallyExclusiveFlags(t *testing.T) {
	t.Parallel()

	scenariosDir := filepath.Join(t.TempDir(), "scenarios")
	if err := os.MkdirAll(filepath.Join(scenariosDir, "training"), 0o755); err != nil {
		t.Fatalf("mkdir scenarios: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenariosDir, "training", "web.yaml"), []byte(validScenarioYAML("web-app-paris", "test")), 0o644); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Scenarios = scenariosDir
	srv := NewServer(ServerConfig{Config: cfg, RunStarter: &fakeStarter{}})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/runs/web-app-paris/start", bytes.NewBufferString(`{"clean":true,"no_destroy":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post start: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestRunsStartBroadcastsToWebSocketClient(t *testing.T) {
	t.Parallel()

	scenariosDir := filepath.Join(t.TempDir(), "scenarios")
	if err := os.MkdirAll(filepath.Join(scenariosDir, "training"), 0o755); err != nil {
		t.Fatalf("mkdir scenarios: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenariosDir, "training", "web.yaml"), []byte(validScenarioYAML("web-app-paris", "test")), 0o644); err != nil {
		t.Fatalf("write scenario: %v", err)
	}

	cfg := config.Default()
	cfg.Paths.Scenarios = scenariosDir
	hub := NewHub()
	starter := &fakeStarter{
		hub: hub,
	}
	srv := NewServer(ServerConfig{Config: cfg, RunStarter: starter, Hub: hub})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{"http://127.0.0.1:5173"},
		},
	})
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/runs/web-app-paris/start", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post start: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket payload: %v", err)
	}
	if !bytes.Contains(payload, []byte(`"type":"log"`)) {
		t.Fatalf("expected log payload, got %s", string(payload))
	}
}

type fakeStarter struct {
	mu      sync.Mutex
	busy    bool
	hub     *Hub
	lastReq StartRunRequest
}

func (f *fakeStarter) StartRun(_ context.Context, req StartRunRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.busy {
		return "", ErrRunBusy
	}
	f.busy = true
	f.lastReq = req
	if f.hub != nil {
		go f.hub.Broadcast([]byte(`{"type":"log","data":{"event":"run_start"}}`))
	}
	return "run-1", nil
}
