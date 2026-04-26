package api

import (
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

type compareResponse struct {
	Run1  string             `json:"run1"`
	Run2  string             `json:"run2"`
	Diffs []compareDiffEntry `json:"diffs"`
}

type compareDiffEntry struct {
	Filename     string `json:"filename"`
	Status       string `json:"status"` // added | removed | modified | unchanged
	UnifiedDiff  string `json:"unified_diff,omitempty"`
}

// handleRunCompare serves GET /api/runs/{scenario}/compare?run1=X&run2=Y.
// It returns file-level diffs between the persisted generated/ snapshots
// of the two runs. Status is one of:
//   - "added": file exists in run2 but not run1
//   - "removed": file exists in run1 but not run2
//   - "modified": file exists in both with different bytes
//   - "unchanged": file exists in both with identical bytes
//
// UnifiedDiff is empty for unchanged files; otherwise it's a standard
// unified diff with up to 3 context lines.
func handleRunCompare(state *serverState, w http.ResponseWriter, r *http.Request, scenarioName string) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	run1 := strings.TrimSpace(r.URL.Query().Get("run1"))
	run2 := strings.TrimSpace(r.URL.Query().Get("run2"))
	if run1 == "" || run2 == "" {
		writeJSONError(w, http.StatusBadRequest, "run1 and run2 query parameters are required")
		return
	}
	for _, id := range []string{run1, run2} {
		if strings.Contains(id, "..") || strings.ContainsAny(id, "/\\") {
			writeJSONError(w, http.StatusBadRequest, "invalid run id")
			return
		}
	}

	files1, err := state.store.ListGeneratedFiles(scenarioName, run1)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "run1 generated files not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	files2, err := state.store.ListGeneratedFiles(scenarioName, run2)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "run2 generated files not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	in1 := make(map[string]struct{}, len(files1))
	for _, f := range files1 {
		in1[f] = struct{}{}
	}
	in2 := make(map[string]struct{}, len(files2))
	for _, f := range files2 {
		in2[f] = struct{}{}
	}
	allFiles := make([]string, 0, len(in1)+len(in2))
	for f := range in1 {
		allFiles = append(allFiles, f)
	}
	for f := range in2 {
		if _, seen := in1[f]; !seen {
			allFiles = append(allFiles, f)
		}
	}
	sort.Strings(allFiles)

	diffs := make([]compareDiffEntry, 0, len(allFiles))
	for _, name := range allFiles {
		_, in1Ok := in1[name]
		_, in2Ok := in2[name]

		var content1, content2 []byte
		if in1Ok {
			content1, err = state.store.ReadGeneratedFile(scenarioName, run1, name)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "read run1 "+name+": "+err.Error())
				return
			}
		}
		if in2Ok {
			content2, err = state.store.ReadGeneratedFile(scenarioName, run2, name)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "read run2 "+name+": "+err.Error())
				return
			}
		}

		entry := compareDiffEntry{Filename: name}
		switch {
		case in1Ok && !in2Ok:
			entry.Status = "removed"
		case !in1Ok && in2Ok:
			entry.Status = "added"
		case bytesEqual(content1, content2):
			entry.Status = "unchanged"
		default:
			entry.Status = "modified"
		}
		if entry.Status != "unchanged" {
			diff, derr := unifiedDiff(name, run1, run2, content1, content2)
			if derr != nil {
				writeJSONError(w, http.StatusInternalServerError, "diff "+name+": "+derr.Error())
				return
			}
			entry.UnifiedDiff = diff
		}
		diffs = append(diffs, entry)
	}

	writeJSON(w, http.StatusOK, compareResponse{Run1: run1, Run2: run2, Diffs: diffs})
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func unifiedDiff(filename, fromLabel, toLabel string, from, to []byte) (string, error) {
	d := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(from)),
		B:        difflib.SplitLines(string(to)),
		FromFile: filename + " (" + fromLabel + ")",
		ToFile:   filename + " (" + toLabel + ")",
		Context:  3,
	}
	return difflib.GetUnifiedDiffString(d)
}
