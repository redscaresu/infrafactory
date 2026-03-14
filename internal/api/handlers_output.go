package api

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func outputHandler(state *serverState) http.HandlerFunc {
	type listResponse struct {
		Files []string `json:"files"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/output/") {
			notImplementedAPIHandler(w, r)
			return
		}
		tail := strings.TrimPrefix(r.URL.Path, "/api/output/")
		tail = strings.TrimPrefix(tail, "/")
		if tail == "" {
			writeJSONError(w, http.StatusNotFound, "scenario not provided")
			return
		}

		parts := strings.Split(tail, "/")
		scenarioName := parts[0]
		root := filepath.Join(state.cfg.Paths.Output, scenarioName)
		if len(parts) == 1 {
			if r.Method != http.MethodGet {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}

			files := make([]string, 0)
			err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					if d.Name() == ".terraform" {
						return fs.SkipDir
					}
					return nil
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				if strings.Contains(rel, "..") || strings.HasPrefix(rel, ".terraform/") {
					return nil
				}
				if strings.HasSuffix(rel, ".tfstate") || strings.Contains(rel, ".tfstate.") || strings.HasSuffix(rel, ".tfplan") || rel == "tfplan" {
					return nil
				}
				files = append(files, rel)
				return nil
			})
			if err != nil {
				if os.IsNotExist(err) {
					writeJSONError(w, http.StatusNotFound, "output not found")
					return
				}
				writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			sort.Strings(files)
			writeJSON(w, http.StatusOK, listResponse{Files: files})
			return
		}

		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		relFile := filepath.Clean(strings.Join(parts[1:], "/"))
		if strings.Contains(relFile, "..") {
			writeJSONError(w, http.StatusForbidden, "path traversal is not allowed")
			return
		}
		target := filepath.Join(root, relFile)
		absRoot, err := filepath.Abs(root)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "resolve output root")
			return
		}
		absTarget, err := filepath.Abs(target)
		if err != nil {
			writeJSONError(w, http.StatusForbidden, "path traversal is not allowed")
			return
		}
		rel, err := filepath.Rel(absRoot, absTarget)
		if err != nil || strings.HasPrefix(rel, "..") {
			writeJSONError(w, http.StatusForbidden, "path traversal is not allowed")
			return
		}

		payload, err := os.ReadFile(absTarget)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSONError(w, http.StatusNotFound, "file not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if shouldFormatRequest(r, relFile) {
			payload, err = state.formatter.Format(r.Context(), relFile, payload)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}
}
