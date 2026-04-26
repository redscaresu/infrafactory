package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/redscaresu/infrafactory/internal/generator"
	"gopkg.in/yaml.v3"
)

type pitfallsResponse struct {
	Providers []pitfallsProviderGroup `json:"providers"`
}

type pitfallsProviderGroup struct {
	Provider string                  `json:"provider"`
	Pitfalls []pitfallsResponseEntry `json:"pitfalls"`
}

type pitfallsResponseEntry struct {
	Resource       string `json:"resource"`
	Rule           string `json:"rule"`
	Source         string `json:"source"`
	DiscoveredFrom string `json:"discovered_from,omitempty"`
}

type pitfallsEditRequest struct {
	Pitfalls []pitfallsResponseEntry `json:"pitfalls"`
}

// pitfallsHandler routes both GET /api/pitfalls (list) and PUT
// /api/pitfalls/{provider} (edit) onto a single dispatcher. The Go mux
// matches /api/pitfalls (exact) and /api/pitfalls/ (prefix) separately;
// see server.go.
func pitfallsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/api/pitfalls":
			listPitfalls(state, w, r)
		case strings.HasPrefix(path, "/api/pitfalls/"):
			provider := strings.TrimPrefix(path, "/api/pitfalls/")
			if provider == "" {
				notImplementedAPIHandler(w, r)
				return
			}
			editPitfalls(state, w, r, provider)
		default:
			notImplementedAPIHandler(w, r)
		}
	}
}

// listPitfalls scans the configured pitfalls directory for *.yaml files,
// parses each as a PitfallsFile, and returns the entries grouped by
// provider (file basename). Providers and their entries are returned in
// deterministic alphabetical order so the UI doesn't have to re-sort.
func listPitfalls(state *serverState, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	dir := strings.TrimSpace(state.cfg.Paths.Pitfalls)
	if dir == "" {
		writeJSON(w, http.StatusOK, pitfallsResponse{Providers: []pitfallsProviderGroup{}})
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, pitfallsResponse{Providers: []pitfallsProviderGroup{}})
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "read pitfalls directory: "+err.Error())
		return
	}

	groups := make([]pitfallsProviderGroup, 0)
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		provider := strings.TrimSuffix(strings.TrimSuffix(name, ".yml"), ".yaml")
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "read pitfalls file "+name+": "+err.Error())
			return
		}
		var file generator.PitfallsFile
		if err := yaml.Unmarshal(data, &file); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "parse pitfalls file "+name+": "+err.Error())
			return
		}
		outEntries := make([]pitfallsResponseEntry, 0, len(file.Pitfalls))
		for _, p := range file.Pitfalls {
			outEntries = append(outEntries, pitfallsResponseEntry{
				Resource:       p.Resource,
				Rule:           p.Rule,
				Source:         p.Source,
				DiscoveredFrom: p.DiscoveredFrom,
			})
		}
		groups = append(groups, pitfallsProviderGroup{
			Provider: provider,
			Pitfalls: outEntries,
		})
	}

	sort.Slice(groups, func(i, j int) bool { return groups[i].Provider < groups[j].Provider })

	writeJSON(w, http.StatusOK, pitfallsResponse{Providers: groups})
}

// editPitfalls writes the given provider's pitfalls YAML atomically. The
// request body must be a JSON object `{"pitfalls": [...]}` whose entries
// each have a non-empty resource and rule. Source defaults to "static"
// when missing. Existing file contents are replaced wholesale.
func editPitfalls(state *serverState, w http.ResponseWriter, r *http.Request, provider string) {
	if r.Method != http.MethodPut {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if strings.Contains(provider, "..") || strings.ContainsAny(provider, "/\\") {
		writeJSONError(w, http.StatusBadRequest, "invalid provider name")
		return
	}
	dir := strings.TrimSpace(state.cfg.Paths.Pitfalls)
	if dir == "" {
		writeJSONError(w, http.StatusFailedDependency, "pitfalls directory is not configured")
		return
	}

	var req pitfallsEditRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "decode body: "+err.Error())
		return
	}

	for i, entry := range req.Pitfalls {
		if strings.TrimSpace(entry.Resource) == "" {
			writeJSONError(w, http.StatusUnprocessableEntity, fmt.Sprintf("pitfalls[%d].resource is required", i))
			return
		}
		if strings.TrimSpace(entry.Rule) == "" {
			writeJSONError(w, http.StatusUnprocessableEntity, fmt.Sprintf("pitfalls[%d].rule is required", i))
			return
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "create pitfalls directory: "+err.Error())
		return
	}

	pf := generator.PitfallsFile{Provider: provider}
	for _, entry := range req.Pitfalls {
		source := entry.Source
		if source == "" {
			source = "static"
		}
		pf.Pitfalls = append(pf.Pitfalls, generator.PitfallEntry{
			Resource:       entry.Resource,
			Rule:           entry.Rule,
			Source:         source,
			DiscoveredFrom: entry.DiscoveredFrom,
		})
	}

	out, err := yaml.Marshal(&pf)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "marshal pitfalls: "+err.Error())
		return
	}

	target := filepath.Join(dir, provider+".yaml")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "write temp pitfalls: "+err.Error())
		return
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		writeJSONError(w, http.StatusInternalServerError, "rename pitfalls: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"provider": provider,
		"count":    len(pf.Pitfalls),
	})
}
