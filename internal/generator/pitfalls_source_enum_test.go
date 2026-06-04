package generator

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPitfallsSourceEnum is the schema ratchet on the `source` field.
// Every entry in pitfalls/<cloud>.yaml must have a `source` field in
// the allowed enum.
//
// Allowed values:
//   - `descriptive` — failure-message echo from ExtractDescriptivePitfall.
//   - `fix`         — extracted from the ADDED side of an HCL diff
//     between a failing iteration and the subsequent passing one
//     (ExtractFixPitfall).
//   - `avoid`       — extracted from the REMOVED side of the same diff
//     (ExtractAvoidPitfall).
//
// The TestPitfallsNoHumanSeeding ratchet already rejects `seed` /
// `static` / empty. This test complements it by fencing the positive
// set: a typo like `avold` or a hand-edited `static-but-renamed` value
// would slip past the seeding ratchet but land here.
//
// Practical effect: once we have committed `avoid` entries (preserved
// through sweep teardown by bin/pitfall-merge), this test guards them
// against accidental deletion via a generic schema mismatch.
func TestPitfallsSourceEnum(t *testing.T) {
	root := repoRoot(t)
	allowed := map[string]bool{
		"descriptive": true,
		"fix":         true,
		"avoid":       true,
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
					t.Errorf("pitfalls/%s.yaml: %s has source=%q — must be one of descriptive / fix / avoid",
						cloud, p.Resource, p.Source)
				}
			}
		})
	}
}
