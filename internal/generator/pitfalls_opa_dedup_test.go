package generator

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPitfallsNoOPADuplication — S82 ratchet against duplication
// between pitfalls/<cloud>.yaml and policies/<cloud>/*.rego.
//
// Context: the dynamic learning loop appends a `learned` pitfall any
// time a recurring failure attaches to a policy_pitfall_conflict-free
// signature. The OPA layer enforces the same shape via its rego
// deny rules, so when both fire we end up with two carriers of the
// same lesson — one in pitfalls/, one in policies/ — and the prompt
// space wastes tokens on a rule the policy already enforces. The
// S82 audit caught three such entries (aws_db_instance.storage_encrypted,
// aws_secretsmanager_secret AWS-managed KMS, google_storage_bucket
// default_kms_key_name).
//
// Detection: extract every `msg := sprintf("...", ...)` literal
// from each policy file; split the format string on `%s`/`%v`/`%d`/`%t`;
// any literal chunk ≥ 30 chars becomes a duplication marker. If a
// pitfall rule contains the marker as a substring, the pitfall is
// duplicative.
//
// Conservative by design: short chunks (<30 chars) won't match —
// they false-positive too easily on common English. Paraphrased
// rules won't match either; this only catches the verbatim-copy
// case the M90 auto-learning loop most frequently produces.
//
// To resolve a failure: delete the pitfall entry. The OPA policy
// is the canonical carrier. If you need to keep both (e.g., the
// policy fires only at one layer and the pitfall reinforces the
// LLM-side), document the divergence and add the rule string to
// the exemption list below — but the default is "remove the
// pitfall."
func TestPitfallsNoOPADuplication(t *testing.T) {
	root := repoRoot(t)
	const minLiteralLen = 30

	for _, cloud := range []string{"aws", "gcp", "scaleway"} {
		t.Run(cloud, func(t *testing.T) {
			policyDir := filepath.Join(root, "policies", cloud)
			literals, err := collectRegoMsgLiterals(policyDir, minLiteralLen)
			if err != nil {
				t.Fatalf("collect rego literals from %s: %v", policyDir, err)
			}
			if len(literals) == 0 {
				t.Fatalf("expected to extract at least one OPA msg literal from %s; got 0", policyDir)
			}

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
				for _, lit := range literals {
					if strings.Contains(p.Rule, lit.text) {
						t.Errorf(
							"pitfalls/%s.yaml: %s carries the OPA-enforced shape from %s — pitfall rule contains %q (from sprintf in %s)",
							cloud, p.Resource, lit.policyFile, lit.text, lit.policyFile,
						)
					}
				}
			}
		})
	}
}

// regoMsgLiteral pairs an extracted literal substring with the
// policy file it came from, so test failures cite the source.
type regoMsgLiteral struct {
	text       string
	policyFile string
}

// sprintfLiteralRe matches Rego `sprintf("<format string>", ...)`.
// Captures the quoted format string. Doesn't handle escaped quotes
// inside the format (none of our policies use them). Permissive
// across whitespace and line breaks between sprintf and its args.
var sprintfLiteralRe = regexp.MustCompile(`sprintf\(\s*"([^"]+)"`)

// formatPlaceholderRe matches the format-string placeholders Rego
// supports (mirrors fmt.Sprintf). Drops them out so we keep only
// the literal text fragments.
var formatPlaceholderRe = regexp.MustCompile(`%[sdvft]`)

func collectRegoMsgLiterals(dir string, minLen int) ([]regoMsgLiteral, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []regoMsgLiteral
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".rego") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		for _, m := range sprintfLiteralRe.FindAllStringSubmatch(string(body), -1) {
			format := m[1]
			for _, chunk := range formatPlaceholderRe.Split(format, -1) {
				trimmed := strings.TrimSpace(chunk)
				if len(trimmed) >= minLen {
					out = append(out, regoMsgLiteral{
						text:       trimmed,
						policyFile: filepath.Join("policies", filepath.Base(dir), e.Name()),
					})
				}
			}
		}
	}
	return out, nil
}
