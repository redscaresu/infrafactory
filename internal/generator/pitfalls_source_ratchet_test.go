package generator

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPitfallsNoHumanSeeding is the M91 ratchet. Every pitfall entry
// in pitfalls/{aws,gcp,scaleway}.yaml must have `source: learned` —
// no `source: seed` and no `source: static` (the two historical
// human-authored shapes).
//
// Rationale: M88's sweep made it impossible to tell the difference
// between "scenario converged because the M86+M90 auto-learning loop
// is working" and "scenario converged because a human pre-loaded
// `source: seed` pitfalls covering the exact failure mode." The
// existing GCP+Scaleway pitfall files had been masking the M90-class
// dead-code branch (stuck→learn never fired) for months because
// seeds kept the LLM converging before stuck-detection could trip.
//
// This guard locks in the policy: pitfalls are run-derived only. If
// a contributor needs the LLM to know X, the failure that teaches
// X must come from a real run — not from human authorship of the
// pitfall file. The M86+M90 loop closes the gap.
//
// To re-enable a temporarily-removed pitfall, run the scenario(s)
// that surface its failure mode and let the auto-learning loop
// rebuild it. If a failure mode can't be auto-learned, the bug is
// in ExtractLearnedPitfall — fix that, not the file.
//
// Counts the absence of seed/static as part of the test; doesn't
// just assert "no seed entries" because absence-of-source-field is
// also a human shape we want to reject.
func TestPitfallsNoHumanSeeding(t *testing.T) {
	root := repoRoot(t)
	for _, cloud := range []string{"aws", "gcp", "scaleway"} {
		t.Run(cloud, func(t *testing.T) {
			path := filepath.Join(root, "pitfalls", cloud+".yaml")
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			var file struct {
				Provider string `yaml:"provider"`
				Pitfalls []struct {
					Resource string `yaml:"resource"`
					Source   string `yaml:"source"`
				} `yaml:"pitfalls"`
			}
			if err := yaml.Unmarshal(payload, &file); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			bySource := map[string]int{}
			for _, p := range file.Pitfalls {
				src := p.Source
				if src == "" {
					src = "<missing>"
				}
				bySource[src]++
			}
			// Sort source keys for stable error output.
			keys := make([]string, 0, len(bySource))
			for k := range bySource {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, src := range keys {
				if src == "learned" {
					continue
				}
				t.Errorf("pitfalls/%s.yaml has %d entries with source=%q — M91 forbids human-authored pitfalls. Delete them and let the M86+M90 auto-learning loop rebuild any genuinely-needed entries from real runs.", cloud, bySource[src], src)
			}
		})
	}
}

// repoRoot resolves the infrafactory repository root from this test
// file's location, mirroring internal/e2e/helpers.go::RepoRoot. Lives
// here as a local helper to avoid pulling in the e2e harness for
// a one-line path resolution.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}
