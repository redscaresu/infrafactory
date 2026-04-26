package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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
	// ParseError, when non-empty, signals that the on-disk yaml for
	// this provider couldn't be parsed and the entries list reflects
	// that (typically empty). The UI should render the bad provider
	// with an inline error rather than crashing the whole page.
	ParseError string `json:"parse_error,omitempty"`
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

// validProviderName matches lower-case provider identifiers like
// "scaleway", "gcp", "fakegcp-internal". Leading dots are rejected so a
// PUT to /api/pitfalls/.bashrc cannot create a hidden file inside the
// pitfalls dir; punctuation outside `[a-z0-9_-]` is rejected so
// path-traversal vectors can't sneak in via Unicode lookalikes. Length
// is capped at 40 chars so an oversized URL surfaces as a 400 client
// error rather than as a filesystem 500 from os.CreateTemp.
var validProviderName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`)

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
				// Trailing slash on a GET is a common client habit;
				// route it to the list handler rather than reject.
				// PUT/POST/etc. with empty provider is still 400.
				if r.Method == http.MethodGet {
					listPitfalls(state, w, r)
					return
				}
				writeJSONError(w, http.StatusBadRequest, "provider name is required")
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
			// One bad file shouldn't blank the whole pitfalls page; surface
			// it as a per-provider parse_error and continue with the rest.
			groups = append(groups, pitfallsProviderGroup{
				Provider:   provider,
				Pitfalls:   []pitfallsResponseEntry{},
				ParseError: err.Error(),
			})
			continue
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
	if !validProviderName.MatchString(provider) {
		writeJSONError(w, http.StatusBadRequest, "invalid provider name")
		return
	}
	dir := strings.TrimSpace(state.cfg.Paths.Pitfalls)
	if dir == "" {
		writeJSONError(w, http.StatusFailedDependency, "pitfalls directory is not configured")
		return
	}

	// Cap payload at 1 MB to match validateScenarioHandler. Use
	// LimitReader+explicit-size-check (rather than MaxBytesReader) so an
	// oversized body returns 413, not the generic 400 MaxBytesReader
	// surfaces via the decoder.
	const maxEditPayloadBytes = 1 << 20
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxEditPayloadBytes+1))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read request body: "+err.Error())
		return
	}
	if len(bodyBytes) > maxEditPayloadBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "edit payload exceeds 1 MB limit")
		return
	}

	var req pitfallsEditRequest
	dec := json.NewDecoder(bytes.NewReader(bodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "decode body: "+err.Error())
		return
	}
	// Reject trailing JSON so a body like `{"pitfalls":[]}{"pitfalls":[…]}`
	// doesn't silently land only the first object. Mirrors the
	// validateScenarioHandler check.
	if dec.More() {
		writeJSONError(w, http.StatusBadRequest, "request body must contain a single JSON object")
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
		source := strings.TrimSpace(entry.Source)
		if source == "" {
			source = "static"
		}
		// Trim before persist so a YAML round-trip can't mangle leading
		// or trailing whitespace, and downstream substring matching on
		// resource type names sees the clean form.
		pf.Pitfalls = append(pf.Pitfalls, generator.PitfallEntry{
			Resource:       strings.TrimSpace(entry.Resource),
			Rule:           strings.TrimSpace(entry.Rule),
			Source:         source,
			DiscoveredFrom: strings.TrimSpace(entry.DiscoveredFrom),
		})
	}

	out, err := yaml.Marshal(&pf)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "marshal pitfalls: "+err.Error())
		return
	}

	target := filepath.Join(dir, provider+".yaml")
	// os.CreateTemp gives us a unique tmp filename so two concurrent PUTs
	// to the same provider can't clobber each other's tmp file before
	// either rename completes; the loser's payload would otherwise
	// silently overwrite the winner's.
	tmp, err := os.CreateTemp(dir, provider+"-*.yaml.tmp")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "create temp pitfalls: "+err.Error())
		return
	}
	tmpPath := tmp.Name()
	// Defer cleanup runs unconditionally; the success path nil-outs the
	// path so cleanup becomes a no-op once the rename committed. This
	// covers the panic-mid-flight case the previous straight-line
	// chain could leak.
	cleanupPath := tmpPath
	defer func() {
		if cleanupPath != "" {
			_ = os.Remove(cleanupPath)
		}
	}()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		writeJSONError(w, http.StatusInternalServerError, "write temp pitfalls: "+err.Error())
		return
	}
	// CreateTemp lands at 0600 by default; pitfalls files in the repo are
	// 0644, so make the in-place mode match what users expect.
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		writeJSONError(w, http.StatusInternalServerError, "chmod temp pitfalls: "+err.Error())
		return
	}
	if err := tmp.Close(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "close temp pitfalls: "+err.Error())
		return
	}
	if err := os.Rename(tmpPath, target); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "rename pitfalls: "+err.Error())
		return
	}
	// Rename succeeded — disarm the cleanup defer so it doesn't unlink
	// the freshly-installed final file.
	cleanupPath = ""

	writeJSON(w, http.StatusOK, map[string]any{
		"provider": provider,
		"count":    len(pf.Pitfalls),
	})
}
