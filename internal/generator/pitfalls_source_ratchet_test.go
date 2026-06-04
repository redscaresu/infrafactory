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
// in ExtractDescriptivePitfall — fix that, not the file.
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
				if src == "descriptive" || src == FixSource || src == AvoidSource {
					// `fix` is N10's auto-derived source —
					// strictly a richer shape of run-derived learning,
					// not a human-authored seed. `avoid`
					// is the N13 deletion-as-fix companion (same
					// provenance, different rule shape). Both whitelisted
					// alongside the legacy `descriptive` tag.
					continue
				}
				t.Errorf("pitfalls/%s.yaml has %d entries with source=%q — M91 forbids human-authored pitfalls. Delete them and let the M86+M90 auto-learning loop rebuild any genuinely-needed entries from real runs.", cloud, bySource[src], src)
			}
		})
	}
}

// TestPitfallsNoMockServerBugSeeds is the N3-T12 companion to M91.
// While M91 asserts "no human-authored pitfalls," this guard asserts
// "no learned pitfall whose Rule matches a mock-actionable signal."
// The two together encode the principle "mock-server gaps are
// tickets, not pitfalls" (see docs/NEXT_SESSION.md § Core design
// principle): a learned entry whose rule echoes a 501 / Plugin-did-
// not-respond / OAuth-escape / 404-from-Describe* failure should
// never have made it into the file — it should have been routed to
// docs/mock-gaps.md by the new IsMockServerBug classifier in
// run_command.go.
//
// Catching these in CI is the enforcement leg of T-12. Before the
// classifier landed (this commit), the existing pitfalls files
// accumulated ~9 such entries across aws + gcp during the 2026-05-31
// sweep. Those are tracked separately under N2 for manual prune.
func TestPitfallsNoMockServerBugSeeds(t *testing.T) {
	root := repoRoot(t)
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
					Rule     string `yaml:"rule"`
				} `yaml:"pitfalls"`
			}
			if err := yaml.Unmarshal(payload, &file); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, p := range file.Pitfalls {
				if IsMockServerBug(p.Rule) {
					sig := FirstMockSignal(p.Rule)
					t.Errorf("pitfalls/%s.yaml: entry for %q has rule matching mock-actionable signal %q — T-12 forbids this. Route the failure to docs/mock-gaps.md instead; the matching mock repo (fakeaws/fakegcp/mockway) should fix the gap at source. Rule was: %.200s",
						cloud, p.Resource, sig, p.Rule)
				}
			}
		})
	}
}

// TestPitfallsLearnedFromDiffSnippetCap is the S55-T3 ratchet. N10's
// trim cap is `snippetMaxBytes = 600` (extractor side) but the rule
// also includes the leading "Fix observed in scenario ..." summary
// (typically ~80-150 bytes). Total rule length is bounded by
// `snippetMaxBytes + 400` per the existing
// TestExtractFixPitfall_SnippetCap test. Enforcing the bound at
// the file level catches any future trim regression that lets an
// uncapped snippet leak through into the on-disk artifact.
func TestPitfallsLearnedFromDiffSnippetCap(t *testing.T) {
	const maxRuleBytes = snippetMaxBytes + 400
	root := repoRoot(t)
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
					Rule     string `yaml:"rule"`
					Source   string `yaml:"source"`
				} `yaml:"pitfalls"`
			}
			if err := yaml.Unmarshal(payload, &file); err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, p := range file.Pitfalls {
				if p.Source != FixSource && p.Source != AvoidSource {
					continue
				}
				if len(p.Rule) > maxRuleBytes {
					t.Errorf("pitfalls/%s.yaml: %q rule is %d bytes (> %d). N10 trim cap leaked.", cloud, p.Resource, len(p.Rule), maxRuleBytes)
				}
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
