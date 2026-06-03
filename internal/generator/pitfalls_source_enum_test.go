package generator

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPitfallsSourceEnum — S94 schema ratchet. Every entry in
// pitfalls/<cloud>.yaml must have a `source` field in the allowed
// enum.
//
// Allowed values:
//   - `learned`              — descriptive failure echo from ExtractLearnedPitfall.
//   - `learned_from_diff`    — N10 prescriptive addition-as-fix.
//   - `learned_from_diff_avoid` — N13 prescriptive deletion-as-fix.
//
// The existing M91 ratchet (`TestPitfallsNoHumanSeeding`) already
// rejects `seed` / `static` / empty. This test complements it by
// fencing the positive set: a typo like `learned_from_diff_avold` or
// a hand-edited `static-but-renamed` value would slip past M91 but
// land here.
//
// Practical effect: once we have committed `learned_from_diff_avoid`
// entries (post-S94's selective discard), this test guards them
// against accidental deletion via a generic schema mismatch.
func TestPitfallsSourceEnum(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]bool{
		"learned":                 true,
		"learned_from_diff":       true,
		"learned_from_diff_avoid": true,
	}
	for _, cloud := range []string{"aws", "gcp", "scaleway"} {
		t.Run(cloud, func(t *testing.T) {
			path := filepath.Join(root, "pitfalls", cloud+".yaml")
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			var file struct {
				Pitfalls []struct {
					Resource string `yaml:"resource"`
					Source   string `yaml:"source"`
				} `yaml:"pitfalls"`
			}
			if err := yaml.Unmarshal(payload, &file); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, p := range file.Pitfalls {
				if !allowed[p.Source] {
					t.Errorf("pitfalls/%s.yaml: %s has source=%q — must be one of learned / learned_from_diff / learned_from_diff_avoid",
						cloud, p.Resource, p.Source)
				}
			}
		})
	}
}
