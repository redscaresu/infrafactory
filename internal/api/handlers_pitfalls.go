package api

import (
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

// listPitfallsHandler serves GET /api/pitfalls. It scans the configured
// pitfalls directory for *.yaml files, parses each as a PitfallsFile,
// and returns the entries grouped by provider (file basename). Providers
// and their entries are returned in deterministic alphabetical order so
// the UI doesn't have to re-sort.
func listPitfallsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
}
