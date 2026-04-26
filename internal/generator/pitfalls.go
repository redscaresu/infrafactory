package generator

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// PitfallEntry represents a single provider pitfall rule.
type PitfallEntry struct {
	Resource       string `yaml:"resource"`
	Rule           string `yaml:"rule"`
	Source         string `yaml:"source"`
	DiscoveredFrom string `yaml:"discovered_from,omitempty"`
}

// PitfallsFile represents the YAML structure of a pitfalls file.
type PitfallsFile struct {
	Provider string         `yaml:"provider"`
	Pitfalls []PitfallEntry `yaml:"pitfalls"`
}

// LoadPitfalls loads pitfalls from the given directory for the specified cloud provider.
// It loads pitfalls/{cloud}.yaml and optionally merges pitfalls/common.yaml.
// Returns a rendered markdown string suitable for prompt injection.
// Returns empty string if no pitfalls file is found (not an error).
func LoadPitfalls(dir, cloud string) (string, error) {
	if dir == "" || cloud == "" {
		return "", nil
	}

	var entries []PitfallEntry

	// Load provider-specific pitfalls.
	providerPath := filepath.Join(dir, cloud+".yaml")
	providerEntries, err := loadPitfallsFile(providerPath)
	if err != nil {
		return "", fmt.Errorf("load pitfalls %q: %w", providerPath, err)
	}
	entries = append(entries, providerEntries...)

	// Load common pitfalls if the file exists.
	commonPath := filepath.Join(dir, "common.yaml")
	commonEntries, err := loadPitfallsFile(commonPath)
	if err != nil {
		return "", fmt.Errorf("load pitfalls %q: %w", commonPath, err)
	}
	entries = append(entries, commonEntries...)

	if len(entries) == 0 {
		return "", nil
	}

	return renderPitfalls(entries), nil
}

// loadPitfallsFile reads and parses a single pitfalls YAML file.
// Returns nil entries and nil error if the file does not exist.
// Caps the file at 1 MB so an accidentally-or-maliciously-huge
// pitfalls YAML can't OOM the generator. Mirrors the API listPitfalls
// handler's symmetric cap.
func loadPitfallsFile(path string) ([]PitfallEntry, error) {
	const maxPitfallsFileBytes = 1 << 20
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPitfallsFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxPitfallsFileBytes {
		return nil, fmt.Errorf("pitfalls file %q exceeds %d bytes", path, maxPitfallsFileBytes)
	}

	var pf PitfallsFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parse %q: %w", path, err)
	}

	return pf.Pitfalls, nil
}

// renderPitfalls renders pitfall entries as a markdown bullet list grouped by resource.
func renderPitfalls(entries []PitfallEntry) string {
	// Group by resource to maintain deterministic output order.
	groups := make(map[string][]string)
	var resourceOrder []string
	for _, e := range entries {
		if _, seen := groups[e.Resource]; !seen {
			resourceOrder = append(resourceOrder, e.Resource)
		}
		groups[e.Resource] = append(groups[e.Resource], e.Rule)
	}
	sort.Strings(resourceOrder)

	var sb strings.Builder
	for _, resource := range resourceOrder {
		for _, rule := range groups[resource] {
			fmt.Fprintf(&sb, "- `%s`: %s\n", resource, strings.TrimSpace(rule))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}
