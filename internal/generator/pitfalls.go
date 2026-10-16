package generator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"gopkg.in/yaml.v3"
)

// PitfallEntry represents a single provider pitfall rule.
type PitfallEntry struct {
	Resource       string `yaml:"resource"`
	Rule           string `yaml:"rule"`
	Source         string `yaml:"source"`
	DiscoveredFrom string `yaml:"discovered_from,omitempty"`
	// LearnedLayer is the layer whose failure taught the rule
	// (mock_deploy, sandbox_deploy, ...); empty for entries that predate
	// it, were not learned from a layer failure, or were learned with no
	// Origin on a run with Layer 3 off, which ran against the mock
	// (generator.LearnedLayer).
	LearnedLayer string `yaml:"learned_layer,omitempty"`

	// ObservedKey is the stable identity of a `source: live` entry: the
	// promotion gate's OWN key for the observation it came from — the
	// status, whether it was version drift, and the normalized detail.
	//
	// It exists because the rule TEXT is not stable. A live rule states
	// its evidence — how many deployments, how long a run — and those
	// numbers grow as the same failure keeps being observed. Matching on
	// the text would therefore append a new entry every time the counters
	// ticked, so the corpus would grow once per cron tick while looking
	// like it was refreshing (S156c, pass 77).
	//
	// It carries the whole of the gate's key rather than the detail alone
	// because the gate keeps `unhealthy` apart from `unreachable`, and a
	// health failure apart from a version mismatch. Persisting anything
	// narrower would collapse distinctions it had just been careful to
	// preserve, and one reproduced failure would overwrite another
	// (pass 85).
	//
	// The key is what does not change: the same failure, however much
	// evidence accumulates for it.
	ObservedKey string `yaml:"observed_key,omitempty"`

	// LastSeen is when the signal behind this rule was last observed,
	// RFC3339. Only `source: live` entries carry it, and only they are
	// retired for going stale (S156a).
	//
	// A rule extracted from a reproducible apply failure does not stop
	// being true because nobody hit it lately; a rule learned from a
	// service that was running does, once that service is fixed.
	LastSeen string `yaml:"last_seen,omitempty"`
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
	entries, err := LoadRenderedPitfalls(dir, cloud)
	if err != nil || len(entries) == 0 {
		return "", err
	}
	return renderPitfalls(entries), nil
}

// LoadRenderedPitfalls returns the entries LoadPitfalls renders into a
// generation prompt: pitfalls/{cloud}.yaml, then pitfalls/common.yaml.
func LoadRenderedPitfalls(dir, cloud string) ([]PitfallEntry, error) {
	if dir == "" || cloud == "" {
		return nil, nil
	}

	var entries []PitfallEntry
	for _, name := range []string{cloud + ".yaml", "common.yaml"} {
		path := filepath.Join(dir, name)
		loaded, err := loadPitfallsFile(path)
		if err != nil {
			return nil, fmt.Errorf("load pitfalls %q: %w", path, err)
		}
		entries = append(entries, loaded...)
	}
	return entries, nil
}

// PitfallID is an entry's stable id across runs: a hash of the cloud,
// the resource and the rule text as rendered. Any edit to the rule is a
// new id, so a run record names the exact rule its prompt carried.
func PitfallID(cloud string, e PitfallEntry) string {
	sum := sha256.Sum256([]byte(cloud + "\x00" + e.Resource + "\x00" + strings.TrimSpace(e.Rule)))
	return hex.EncodeToString(sum[:8])
}

// MatchBrokenPitfall returns the id of the first rendered entry for the
// failure's resource type whose rule the HCL under dir breaks, or "".
// The failure's type comes from its resource address, else the first
// address in its detail.
//
// Only an avoid rule (ParseAvoidRule) can be judged against HCL: it is
// broken when a block of its type sets every attribute it names to
// something other than a literal off value. Any other rule never matches.
func MatchBrokenPitfall(dir, cloud, failureResource, failureDetail string, rendered []PitfallEntry) (string, error) {
	address := failureResource
	if address == "" {
		address = firstResourceAddress(failureDetail)
	}
	resourceType, _, ok := splitAddress(address)
	if !ok {
		return "", nil
	}
	bodies, err := resourceBodies(dir, resourceType)
	if err != nil {
		return "", fmt.Errorf("read generated HCL %s: %w", dir, err)
	}
	for _, e := range rendered {
		resource, attrs, ok := ParseAvoidRule(e.Rule)
		if !ok || e.Resource != resourceType || resource != resourceType {
			continue
		}
		if slices.ContainsFunc(bodies, func(b *hclsyntax.Body) bool { return setsAttributesOn(b, attrs) }) {
			return PitfallID(cloud, e), nil
		}
	}
	return "", nil
}

// resourceBodies parses every *.tf file under dir and returns the body of
// each resource block of resourceType. A file with syntax errors yields
// the blocks HCL could still parse.
func resourceBodies(dir, resourceType string) ([]*hclsyntax.Body, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		return nil, err
	}
	var bodies []*hclsyntax.Body
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		file, _ := hclsyntax.ParseConfig(src, path, hcl.InitialPos)
		if file == nil {
			continue
		}
		body, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			continue
		}
		for _, b := range body.Blocks {
			if b.Type == "resource" && len(b.Labels) == 2 && b.Labels[0] == resourceType {
				bodies = append(bodies, b.Body)
			}
		}
	}
	return bodies, nil
}

// setsAttributesOn reports whether body sets every attribute, none of
// them to a literal off value.
func setsAttributesOn(body *hclsyntax.Body, attrs []string) bool {
	return setsEveryAttribute(body, attrs) &&
		!slices.ContainsFunc(attrs, func(a string) bool { return literalOff(body.Attributes[a].Expr) })
}

// LoadPitfallEntries reads pitfalls/<cloud>.yaml and returns the
// parsed entries. Empty result + nil error if the file doesn't
// exist (a fresh checkout has no learned pitfalls yet).
//
// Used by N8's DetectPolicyConflict in run_command.go to fetch the
// pitfall list the LLM's HCL is being compared against.
func LoadPitfallEntries(dir, cloud string) ([]PitfallEntry, error) {
	if dir == "" || cloud == "" {
		return nil, nil
	}
	path := filepath.Join(dir, cloud+".yaml")
	return loadPitfallsFile(path)
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
			sb.WriteString(pitfallLine(resource, rule) + "\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// pitfallLine is how one entry reads in a prompt.
func pitfallLine(resource, rule string) string {
	return fmt.Sprintf("- `%s`: %s", resource, strings.TrimSpace(rule))
}

// PitfallsInPrompts returns the entries whose rendered line appears in
// any of the phases' prompts: what the generator was actually shown.
func PitfallsInPrompts(entries []PitfallEntry, phases []PhaseResult) []PitfallEntry {
	var shown []PitfallEntry
	for _, e := range entries {
		line := []byte(pitfallLine(e.Resource, e.Rule))
		if slices.ContainsFunc(phases, func(p PhaseResult) bool { return bytes.Contains(p.Prompt, line) }) {
			shown = append(shown, e)
		}
	}
	return shown
}
