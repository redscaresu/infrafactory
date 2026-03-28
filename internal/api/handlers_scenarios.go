package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

type scenariosListResponse struct {
	Groups []scenarioGroup `json:"groups"`
}

type scenarioGroup struct {
	Name      string             `json:"name"`
	Scenarios []scenarioListItem `json:"scenarios"`
}

type scenarioListItem struct {
	Name        string              `json:"name"`
	Path        string              `json:"path"`
	Description string              `json:"description"`
	LastRun     *runMetadataSummary `json:"last_run,omitempty"`
}

type runMetadataSummary struct {
	RunID          string `json:"run_id"`
	Status         string `json:"status"`
	TerminalReason string `json:"terminal_reason,omitempty"`
}

type scenarioDetailResponse struct {
	Name        string                         `json:"name"`
	Path        string                         `json:"path"`
	Description string                         `json:"description"`
	RawYAML     string                         `json:"raw_yaml"`
	Resources   scenario.Resources             `json:"resources"`
	Constraints map[string]any                 `json:"constraints,omitempty"`
	Criteria    []scenario.AcceptanceCriterion `json:"criteria"`
}

func listScenariosHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		groupsMap := map[string][]scenarioListItem{}
		err := filepath.WalkDir(state.cfg.Paths.Scenarios, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".yaml") && !strings.HasSuffix(d.Name(), ".yml") {
				return nil
			}

			rel, err := filepath.Rel(state.cfg.Paths.Scenarios, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)

			sc, _, err := loadScenarioFile(path, state.scenarioSchemaPathCandidates())
			if err != nil {
				return err
			}

			scenarioPath := strings.TrimSuffix(strings.TrimSuffix(rel, ".yaml"), ".yml")
			group := "root"
			if idx := strings.IndexRune(scenarioPath, '/'); idx > 0 {
				group = scenarioPath[:idx]
			}

			item := scenarioListItem{
				Name:        sc.Name,
				Path:        scenarioPath,
				Description: sc.Description,
				LastRun:     latestRunSummary(state.store, sc.Name),
			}
			groupsMap[group] = append(groupsMap[group], item)
			return nil
		})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		groupNames := make([]string, 0, len(groupsMap))
		for name := range groupsMap {
			groupNames = append(groupNames, name)
		}
		sort.Strings(groupNames)

		resp := scenariosListResponse{
			Groups: make([]scenarioGroup, 0, len(groupNames)),
		}
		for _, name := range groupNames {
			items := groupsMap[name]
			sort.Slice(items, func(i, j int) bool {
				return items[i].Path < items[j].Path
			})
			resp.Groups = append(resp.Groups, scenarioGroup{Name: name, Scenarios: items})
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func scenarioByPathHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relPath, ok := parseTailPath(r.URL.Path, "/api/scenarios/")
		if !ok {
			writeJSONError(w, http.StatusNotFound, "scenario path not provided")
			return
		}
		runModeRequest := false
		layer3StatusRequest := false
		if strings.HasSuffix(relPath, "/run-mode") {
			runModeRequest = true
			relPath = strings.TrimSuffix(relPath, "/run-mode")
		} else if strings.HasSuffix(relPath, "/layer3-status") {
			layer3StatusRequest = true
			relPath = strings.TrimSuffix(relPath, "/layer3-status")
		}

		scenarioFile, err := resolveScenarioFile(state.cfg.Paths.Scenarios, relPath)
		if err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}

		switch r.Method {
		case http.MethodGet:
			if runModeRequest {
				handleGetScenarioRunMode(w, r.Context(), state, relPath, scenarioFile)
				return
			}
			if layer3StatusRequest {
				handleGetScenarioLayer3Status(w, state, relPath, scenarioFile)
				return
			}
			handleGetScenarioByPath(w, state, relPath, scenarioFile)
		case http.MethodPut:
			if runModeRequest || layer3StatusRequest {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			handlePutScenarioByPath(w, r, state, scenarioFile)
		default:
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

func handleGetScenarioLayer3Status(w http.ResponseWriter, state *serverState, relPath, scenarioFile string) {
	if _, _, err := loadScenarioFile(scenarioFile, state.scenarioSchemaPathCandidates()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "scenario not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	missing := make([]string, 0, 2)
	if strings.TrimSpace(os.Getenv("SCW_ACCESS_KEY")) == "" {
		missing = append(missing, "SCW_ACCESS_KEY")
	}
	if strings.TrimSpace(os.Getenv("SCW_SECRET_KEY")) == "" {
		missing = append(missing, "SCW_SECRET_KEY")
	}
	ready := len(missing) == 0
	detail := "Layer 3 credentials ready"
	if !ready {
		detail = "Missing " + strings.Join(missing, ", ")
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":                   filepath.Base(relPath),
		"path":                   relPath,
		"config_default_enabled": state.cfg.Validation.Layers.SandboxDeploy.Enabled,
		"credentials_ready":      ready,
		"missing_credentials":    missing,
		"ready":                  ready,
		"detail":                 detail,
	})
}

func handleGetScenarioByPath(w http.ResponseWriter, state *serverState, relPath, scenarioFile string) {
	sc, rawYAML, err := loadScenarioFile(scenarioFile, state.scenarioSchemaPathCandidates())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "scenario not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, scenarioDetailResponse{
		Name:        sc.Name,
		Path:        relPath,
		Description: sc.Description,
		RawYAML:     string(rawYAML),
		Resources:   sc.Resources,
		Constraints: sc.Constraints,
		Criteria:    sc.AcceptanceCriteria,
	})
}

func handlePutScenarioByPath(w http.ResponseWriter, r *http.Request, state *serverState, scenarioFile string) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("read request body: %v", err))
		return
	}

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("scenario-validate-%d.yaml", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, payload, 0o600); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("write temp scenario: %v", err))
		return
	}
	defer os.Remove(tmpFile)

	_, _, err = loadScenarioFile(tmpFile, state.scenarioSchemaPathCandidates())
	if err != nil {
		var validationErr *scenario.ValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"errors": validationErr.Violations,
			})
			return
		}
		if errors.Is(err, scenario.ErrMalformedScenario) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := os.WriteFile(scenarioFile, payload, 0o644); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("write scenario file: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func latestRunSummary(store *runstore.FilesystemStore, scenarioName string) *runMetadataSummary {
	runs, err := store.ListRuns(scenarioName)
	if err != nil || len(runs) == 0 {
		return nil
	}
	latest := runs[0]
	for _, run := range runs[1:] {
		if run.StartedAt.After(latest.StartedAt) || (run.StartedAt.Equal(latest.StartedAt) && run.RunID > latest.RunID) {
			latest = run
		}
	}
	return &runMetadataSummary{
		RunID:          latest.RunID,
		Status:         latest.Status,
		TerminalReason: latest.TerminalReason,
	}
}

func handleGetScenarioRunMode(w http.ResponseWriter, ctx context.Context, state *serverState, relPath, scenarioFile string) {
	sc, _, err := loadScenarioFile(scenarioFile, state.scenarioSchemaPathCandidates())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "scenario not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if state.mockState == nil {
		writeJSONError(w, http.StatusNotImplemented, "mock state detection is not configured")
		return
	}

	statePayload, err := state.mockState.State(ctx)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hasMockResources, err := apiMockStateHasResources(statePayload)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("decode mock state for run mode detection: %v", err))
		return
	}
	hasTFState := apiTFStateExists(filepath.Join(state.cfg.Paths.Output, sc.Name))
	previousRunID, err := state.store.LatestSuccessfulRunID(sc.Name)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	missing := make([]string, 0, 3)
	if !hasMockResources {
		missing = append(missing, "mockway state")
	}
	if !hasTFState {
		missing = append(missing, "terraform.tfstate")
	}
	if previousRunID == "" {
		missing = append(missing, "previous successful run")
	}

	mode := "incremental"
	reason := "auto-detected from mockway state, terraform.tfstate, and previous successful run"
	if len(missing) > 0 {
		mode = "clean"
		reason = "missing " + strings.Join(missing, ", ")
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":                        sc.Name,
		"path":                        relPath,
		"mode":                        mode,
		"reason":                      reason,
		"previous_run_id":             previousRunID,
		"has_mock_resources":          hasMockResources,
		"has_tfstate":                 hasTFState,
		"has_previous_successful_run": previousRunID != "",
	})
}

func apiMockStateHasResources(payload []byte) (bool, error) {
	var state map[string]any
	if err := json.Unmarshal(payload, &state); err != nil {
		return false, err
	}
	for _, rootNode := range state {
		rootMap, ok := rootNode.(map[string]any)
		if !ok {
			continue
		}
		for _, value := range rootMap {
			items, ok := value.([]any)
			if ok && len(items) > 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

func apiTFStateExists(outputDir string) bool {
	_, err := os.Stat(filepath.Join(outputDir, "terraform.tfstate"))
	return err == nil
}

func parseTailPath(fullPath, prefix string) (string, bool) {
	if !strings.HasPrefix(fullPath, prefix) {
		return "", false
	}
	tail := strings.TrimPrefix(fullPath, prefix)
	tail = strings.TrimPrefix(tail, "/")
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return "", false
	}
	return filepath.ToSlash(tail), true
}

func resolveScenarioFile(root, relPath string) (string, error) {
	decodedRel, err := url.PathUnescape(relPath)
	if err != nil {
		return "", fmt.Errorf("invalid scenario path encoding")
	}
	cleanRel := filepath.Clean(decodedRel)
	if cleanRel == "." || cleanRel == "" {
		return "", fmt.Errorf("invalid scenario path")
	}
	if strings.Contains(cleanRel, "..") {
		return "", fmt.Errorf("path traversal is not allowed")
	}

	filePath := cleanRel
	if filepath.Ext(filePath) == "" {
		filePath += ".yaml"
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve scenarios root: %w", err)
	}
	absTarget, err := filepath.Abs(filepath.Join(absRoot, filePath))
	if err != nil {
		return "", fmt.Errorf("resolve scenario path: %w", err)
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return "", fmt.Errorf("resolve scenario relative path: %w", err)
	}
	if strings.HasPrefix(rel, "..") || rel == "." {
		return "", fmt.Errorf("path traversal is not allowed")
	}
	return absTarget, nil
}

func loadScenarioFile(path string, schemaCandidates []string) (scenario.Scenario, []byte, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return scenario.Scenario{}, nil, err
	}
	var lastErr error
	for _, schemaPath := range schemaCandidates {
		if _, err := os.Stat(schemaPath); err != nil {
			lastErr = err
			continue
		}
		sc, err := scenario.LoadWithSchema(path, schemaPath)
		if err == nil {
			return sc, payload, nil
		}
		return scenario.Scenario{}, nil, err
	}
	if lastErr != nil {
		return scenario.Scenario{}, nil, fmt.Errorf("locate scenario schema: %w", lastErr)
	}
	return scenario.Scenario{}, nil, fmt.Errorf("locate scenario schema: no schema paths configured")
}
