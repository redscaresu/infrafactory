package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPromptsNoOPAPolicyCitations — S99 ratchet against prompt rules
// that explicitly name an OPA policy file as their enforcement
// mechanism.
//
// S82's `TestPitfallsNoOPADuplication` catches verbatim OPA-msg
// duplication in `pitfalls/<cloud>.yaml`. It misses a related shape
// in `prompts/<cloud>/*.md`: a prompt rule that names an existing
// `.rego` policy by name and says the policy enforces the rule. GCP
// phase3 rule #13 was exactly that shape:
//
//	"The `region_restriction` OPA policy enforces this."
//
// Category B per ADR-0018 — load-bearing only because the prompt
// reminds the LLM, but OPA already catches violations at validate-
// time. The prompt rule wastes tokens.
//
// Detection: for each cloud, collect basenames of every
// `.rego` file in `policies/<cloud>/`. Scan that cloud's
// `prompts/<cloud>/*.md` files for patterns that tie a policy name
// to "OPA" or ".rego":
//
//   - `<name> OPA policy`
//   - `<name>` OPA policy
//   - OPA `<name>` policy
//   - OPA <name> policy
//   - <name>.rego
//
// Tight by design: paraphrased mentions ("the OPA policy that
// restricts regions") aren't matched — those are Category C
// because the prompt isn't claiming OPA enforces it BY NAME.
//
// To resolve a failure: retire the prompt rule (OPA already
// enforces it) OR rephrase without the policy name + the "OPA"/
// ".rego" anchor.
func TestPromptsNoOPAPolicyCitations(t *testing.T) {
	root := repoRoot(t)

	for _, cloud := range []string{"aws", "gcp", "scaleway"} {
		t.Run(cloud, func(t *testing.T) {
			policyDir := filepath.Join(root, "policies", cloud)
			promptDir := filepath.Join(root, "prompts", cloud)

			regoNames, err := collectRegoBasenames(policyDir)
			if err != nil {
				t.Fatalf("collect rego basenames from %s: %v", policyDir, err)
			}
			if len(regoNames) == 0 {
				t.Fatalf("expected at least one .rego in %s; got 0", policyDir)
			}

			promptEntries, err := os.ReadDir(promptDir)
			if err != nil {
				t.Fatalf("read %s: %v", promptDir, err)
			}

			for _, pe := range promptEntries {
				if pe.IsDir() || !strings.HasSuffix(pe.Name(), ".md") {
					continue
				}
				path := filepath.Join(promptDir, pe.Name())
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read %s: %v", path, err)
				}
				text := string(body)
				for _, name := range regoNames {
					for _, pat := range citationPatterns(name) {
						if strings.Contains(text, pat) {
							t.Errorf(
								"prompts/%s/%s mentions OPA policy %q via %q — Category B per ADR-0018; retire the rule (OPA already enforces) or rephrase without naming the policy",
								cloud, pe.Name(), name, pat,
							)
						}
					}
				}
			}
		})
	}
}

// citationPatterns returns the prompt-side substrings that anchor a
// policy name to OPA. Returning a slice instead of a single regex so
// failure messages can cite the exact offending pattern.
func citationPatterns(regoName string) []string {
	return []string{
		"`" + regoName + "` OPA policy",
		"`" + regoName + "` policy enforces",
		"OPA `" + regoName + "` policy",
		"OPA " + regoName + " policy",
		regoName + ".rego",
	}
}

func collectRegoBasenames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".rego") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".rego"))
	}
	return out, nil
}
