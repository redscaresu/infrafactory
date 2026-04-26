package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/redscaresu/infrafactory/internal/runstore"
)

func listAllRunsHandler(state *serverState) http.HandlerFunc {
	type response struct {
		Runs []runstore.RunMetadata `json:"runs"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		scenarios, err := state.store.ListScenarios()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		all := make([]runstore.RunMetadata, 0)
		for _, scenarioName := range scenarios {
			runs, err := state.store.ListRuns(scenarioName)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			all = append(all, runs...)
		}
		sort.Slice(all, func(i, j int) bool {
			return all[i].RunID > all[j].RunID
		})
		writeJSON(w, http.StatusOK, response{Runs: all})
	}
}

func runsByScenarioHandler(state *serverState) http.HandlerFunc {
	type response struct {
		Runs []runstore.RunMetadata `json:"runs"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/runs/") {
			notImplementedAPIHandler(w, r)
			return
		}

		tail := strings.TrimPrefix(r.URL.Path, "/api/runs/")
		parts := strings.Split(strings.Trim(tail, "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			notImplementedAPIHandler(w, r)
			return
		}

		scenarioName := parts[0]
		if strings.Contains(scenarioName, "..") || strings.ContainsAny(scenarioName, "/\\") {
			writeJSONError(w, http.StatusBadRequest, "invalid scenario name")
			return
		}
		if len(parts) == 2 && parts[1] == "start" && r.Method == http.MethodPost {
			startRunHandler(state, w, r, scenarioName)
			return
		}

		if len(parts) == 2 && parts[1] == "compare" {
			handleRunCompare(state, w, r, scenarioName)
			return
		}

		if len(parts) == 1 {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			runs, err := state.store.ListRuns(scenarioName)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, response{Runs: runs})
			return
		}

		if len(parts) == 2 {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			meta, err := state.store.ReadRunMetadata(scenarioName, parts[1])
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					writeJSONError(w, http.StatusNotFound, "run not found")
					return
				}
				writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, meta)
			return
		}

		if len(parts) == 3 && parts[2] == "bundle.zip" {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			handleRunBundle(state, w, scenarioName, parts[1])
			return
		}

		if len(parts) == 3 && parts[2] == "artifacts.zip" {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			handleRunArtifactsArchive(state, w, scenarioName, parts[1])
			return
		}

		if len(parts) == 3 && parts[2] == "log" {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			handleRunLog(state, w, scenarioName, parts[1])
			return
		}

		if len(parts) == 3 && parts[2] == "plan" {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			handleRunArtifact(state, w, scenarioName, parts[1], "plan.txt", "plan not found", "text/plain; charset=utf-8")
			return
		}

		if len(parts) == 3 && parts[2] == "baseline" {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			handleRunArtifact(state, w, scenarioName, parts[1], "baseline_state.json", "baseline state not found", "application/json")
			return
		}

		if len(parts) >= 3 && parts[2] == "files" {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			handleRunFiles(state, w, r, parts)
			return
		}

		if len(parts) == 3 && parts[2] == "iterations" {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			handleRunIterations(state, w, scenarioName, parts[1])
			return
		}

		if len(parts) >= 4 && parts[2] == "iterations" {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			handleRunIterationSubresource(state, w, r, scenarioName, parts[1], parts[3:])
			return
		}

		notImplementedAPIHandler(w, r)
	}
}

func handleRunIterations(state *serverState, w http.ResponseWriter, scenarioName, runID string) {
	type response struct {
		Iterations []int `json:"iterations"`
	}

	iterations, err := state.store.ListIterations(scenarioName, runID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response{Iterations: iterations})
}

func handleRunIterationSubresource(state *serverState, w http.ResponseWriter, r *http.Request, scenarioName, runID string, tail []string) {
	iteration, err := strconv.Atoi(tail[0])
	if err != nil || iteration < 1 {
		writeJSONError(w, http.StatusBadRequest, "invalid iteration")
		return
	}

	if len(tail) == 1 {
		payload, err := state.store.ReadIterationArtifact(scenarioName, runID, iteration)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				writeJSONError(w, http.StatusNotFound, "iteration not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
		return
	}

	if len(tail) >= 2 && tail[1] == "files" {
		handleIterationFiles(state, w, r, scenarioName, runID, iteration, tail[2:])
		return
	}

	notImplementedAPIHandler(w, r)
}

func handleIterationFiles(state *serverState, w http.ResponseWriter, r *http.Request, scenarioName, runID string, iteration int, tail []string) {
	type listResponse struct {
		Files []string `json:"files"`
	}

	if len(tail) == 0 {
		files, err := state.store.ListIterationGeneratedFiles(scenarioName, runID, iteration)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				writeJSONError(w, http.StatusNotFound, "iteration generated files not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, listResponse{Files: files})
		return
	}

	relPath := filepath.Clean(strings.Join(tail, "/"))
	if strings.Contains(relPath, "..") {
		writeJSONError(w, http.StatusForbidden, "path traversal is not allowed")
		return
	}

	payload, err := state.store.ReadIterationGeneratedFile(scenarioName, runID, iteration, relPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "iteration generated file not found")
			return
		}
		if strings.Contains(err.Error(), "invalid generated file path") {
			writeJSONError(w, http.StatusForbidden, "path traversal is not allowed")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if shouldFormatRequest(r, relPath) {
		payload, err = state.formatter.Format(r.Context(), relPath, payload)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func handleRunBundle(state *serverState, w http.ResponseWriter, scenarioName, runID string) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	finalFiles, err := state.store.ListGeneratedFiles(scenarioName, runID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, file := range finalFiles {
		payload, err := state.store.ReadGeneratedFile(scenarioName, runID, file)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := addZipFile(zw, filepath.ToSlash(filepath.Join("generated", file)), payload); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	iterations, err := state.store.ListIterations(scenarioName, runID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, iteration := range iterations {
		files, err := state.store.ListIterationGeneratedFiles(scenarioName, runID, iteration)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, file := range files {
			payload, err := state.store.ReadIterationGeneratedFile(scenarioName, runID, iteration, file)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "read iteration generated file")
				return
			}
			zipPath := filepath.ToSlash(filepath.Join("iterations", strconv.Itoa(iteration), "generated", file))
			if err := addZipFile(zw, zipPath, payload); err != nil {
				writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	if err := zw.Close(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+scenarioName+"-"+runID+"-iac.zip\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func handleRunLog(state *serverState, w http.ResponseWriter, scenarioName, runID string) {
	payload, err := state.store.ReadRunLog(scenarioName, runID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "run log not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func handleRunArtifact(state *serverState, w http.ResponseWriter, scenarioName, runID, name, notFoundMessage, contentType string) {
	payload, err := state.store.ReadRunArtifact(scenarioName, runID, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, notFoundMessage)
			return
		}
		if strings.Contains(err.Error(), "invalid generated file path") {
			writeJSONError(w, http.StatusForbidden, "path traversal is not allowed")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func handleRunArtifactsArchive(state *serverState, w http.ResponseWriter, scenarioName, runID string) {
	if _, err := state.store.ReadRunMetadata(scenarioName, runID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "run not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	runRoot := filepath.Join(state.store.Root, scenarioName, runID)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.WalkDir(runRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(runRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "..") {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return addZipFile(zw, rel, payload)
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := zw.Close(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+scenarioName+"-"+runID+"-artifacts.zip\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

func addZipFile(zw *zip.Writer, name string, payload []byte) error {
	writer, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write(payload)
	return err
}

func handleRunFiles(state *serverState, w http.ResponseWriter, r *http.Request, parts []string) {
	type listResponse struct {
		Files []string `json:"files"`
	}

	scenarioName := parts[0]
	runID := parts[1]
	if len(parts) == 3 {
		files, err := state.store.ListGeneratedFiles(scenarioName, runID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				writeJSONError(w, http.StatusNotFound, "generated files not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, listResponse{Files: files})
		return
	}

	relPath := filepath.Clean(strings.Join(parts[3:], "/"))
	if strings.Contains(relPath, "..") {
		writeJSONError(w, http.StatusForbidden, "path traversal is not allowed")
		return
	}

	payload, err := state.store.ReadGeneratedFile(scenarioName, runID, relPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "generated file not found")
			return
		}
		if strings.Contains(err.Error(), "invalid generated file path") {
			writeJSONError(w, http.StatusForbidden, "path traversal is not allowed")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if shouldFormatRequest(r, relPath) {
		payload, err = state.formatter.Format(r.Context(), relPath, payload)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func startRunHandler(state *serverState, w http.ResponseWriter, r *http.Request, scenarioName string) {
	if state.runStarter == nil {
		writeJSONError(w, http.StatusNotImplemented, "run executor is not configured")
		return
	}

	scenarioRelPath, err := findScenarioPathByName(state, scenarioName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "scenario not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	req := StartRunRequest{
		ScenarioName: scenarioName,
		ScenarioPath: scenarioRelPath,
	}
	if r.Body != nil {
		defer r.Body.Close()
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid json body")
				return
			}
		}
	}
	req.ScenarioName = scenarioName
	req.ScenarioPath = scenarioRelPath
	if req.Clean && req.NoDestroy {
		writeJSONError(w, http.StatusUnprocessableEntity, "clean and no_destroy are mutually exclusive")
		return
	}

	runID, err := state.runStarter.StartRun(r.Context(), req)
	if err != nil {
		if errors.Is(err, ErrRunBusy) {
			writeJSONError(w, http.StatusConflict, "run already in progress")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID})
}

func findScenarioPathByName(state *serverState, scenarioName string) (string, error) {
	root := state.cfg.Paths.Scenarios
	var found string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".yaml") && !strings.HasSuffix(d.Name(), ".yml") {
			return nil
		}

		sc, _, err := loadScenarioFile(path, state.scenarioSchemaPathCandidates())
		if err != nil {
			return err
		}
		if sc.Name != scenarioName {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(strings.TrimSuffix(strings.TrimSuffix(rel, ".yaml"), ".yml"))
		found = rel
		return io.EOF
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if found == "" {
		return "", os.ErrNotExist
	}
	return found, nil
}
