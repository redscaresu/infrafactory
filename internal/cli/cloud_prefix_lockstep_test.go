package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// TestCloudPrefixLockstep enforces ADR-0021: the three sites that
// gate auto-learning by cloud prefix MUST agree on the set of clouds.
// Adding a new cloud means updating all three; missing one silently
// breaks pitfall learning for that cloud (the very bug S118 fixed
// when fakegenesys arrived and genesys pitfalls couldn't be extracted).
//
// Sites:
//
//  1. internal/generator/pitfalls_learn.go::resourceNameRe
//  2. internal/generator/prescriptive_extractor.go::addressRe
//  3. internal/cli/run_command.go::pitfallResourceMatchesCloud
//
// Contract:
//
//   - Sites 1 and 3 must have the IDENTICAL cloud-prefix set.
//   - Site 2 (addressRe) may be a SUPERSET — it parses provider-less
//     resources too (e.g. random_*) which aren't cloud-specific.
//   - Any extra prefix in site 2 must be in the permitted-extras list
//     below (currently just "random"). Add to the list when a new
//     non-cloud provider needs to be parseable but not learned-from.
//
// The failure mode this prevents: a fifth cloud's resourceNameRe and
// pitfallResourceMatchesCloud diverge silently. Auto-learning produces
// empty resource names for the new cloud, ExtractResourceFromDetail
// returns "", and the system fails to learn pitfalls without any
// surfaced error. S118 surfaced because the user pushed back on
// "expected cold-start" framing; this audit makes the regression
// detectable in CI before merge.
func TestCloudPrefixLockstep(t *testing.T) {
	root := repoRoot(t)

	site1 := readPrefixesFromAlternation(t,
		filepath.Join(root, "internal/generator/pitfalls_learn.go"),
		`resourceNameRe\s*=\s*regexp\.MustCompile\(\x60\(\(\?:([^)]+)\)`,
		"pitfalls_learn.go::resourceNameRe")

	site2 := readPrefixesFromAlternation(t,
		filepath.Join(root, "internal/generator/prescriptive_extractor.go"),
		`addressRe\s*=\s*regexp\.MustCompile\(\x60\(\(\?:([^)]+)\)`,
		"prescriptive_extractor.go::addressRe")

	site3 := readPrefixesFromSwitchCases(t,
		filepath.Join(root, "internal/cli/run_command.go"),
		"pitfallResourceMatchesCloud")

	// Site 1 and Site 3 must be identical.
	if !setEqual(site1, site3) {
		t.Errorf("cloud-prefix lockstep violation between site 1 and site 3:\n"+
			"  pitfalls_learn.go::resourceNameRe         = %v\n"+
			"  run_command.go::pitfallResourceMatchesCloud = %v\n"+
			"  missing from site 3: %v\n"+
			"  extra in site 3:     %v\n"+
			"See ADR-0021 — adding a new cloud requires updating both.",
			sorted(site1), sorted(site3),
			diff(site1, site3), diff(site3, site1))
	}

	// Site 2 must be a superset of site 1, with only "random" allowed
	// as the extra.
	permittedExtras := map[string]struct{}{"random": {}}

	missingFromSite2 := diff(site1, site2)
	if len(missingFromSite2) > 0 {
		t.Errorf("cloud-prefix lockstep violation: site 2 missing prefix(es):\n"+
			"  pitfalls_learn.go::resourceNameRe         = %v\n"+
			"  prescriptive_extractor.go::addressRe      = %v\n"+
			"  missing from addressRe: %v\n"+
			"See ADR-0021 — addressRe must include every cloud prefix from resourceNameRe.",
			sorted(site1), sorted(site2), missingFromSite2)
	}

	extrasInSite2 := diff(site2, site1)
	for _, p := range extrasInSite2 {
		if _, ok := permittedExtras[p]; !ok {
			t.Errorf("cloud-prefix lockstep violation: addressRe has unpermitted extra prefix %q.\n"+
				"  If %q is a real cloud, add it to site 1 (resourceNameRe) and site 3 (pitfallResourceMatchesCloud) too.\n"+
				"  If %q is a non-cloud provider like random_*, add it to permittedExtras in this test.\n"+
				"See ADR-0021.",
				p, p, p)
		}
	}

	t.Logf("cloud-prefix lockstep OK: %d clouds (%v) consistent across 3 sites; addressRe extras: %v",
		len(site1), sorted(site1), extrasInSite2)
}

// readPrefixesFromAlternation extracts the alternation group from a
// regex compiled in the source — e.g. the `scaleway|google|aws|...`
// inside `regexp.MustCompile(\`((?:scaleway|google|aws|...)_...\`)`.
//
// Strips a trailing "_" if present — the prefix list is canonical
// (no trailing separator) so it compares cleanly across sites.
func readPrefixesFromAlternation(t *testing.T, path, pattern, label string) map[string]struct{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	re := regexp.MustCompile(pattern)
	m := re.FindSubmatch(raw)
	if len(m) < 2 {
		t.Fatalf("could not locate alternation group at %s — has the regex form changed? Pattern was %q", label, pattern)
	}
	alt := strings.TrimSpace(string(m[1]))
	out := map[string]struct{}{}
	for _, p := range strings.Split(alt, "|") {
		p = strings.TrimSpace(p)
		// Strip trailing underscore if present so the prefix is
		// canonical across the 3 sites.
		p = strings.TrimSuffix(p, "_")
		if p == "" {
			continue
		}
		out[p] = struct{}{}
	}
	if len(out) == 0 {
		t.Fatalf("zero prefixes parsed from %s — regex likely matched empty alternation", label)
	}
	return out
}

// readPrefixesFromSwitchCases parses the switch-on-cloud statement in
// pitfallResourceMatchesCloud and extracts the resource prefix each
// case checks for. The contract is the SET of resource prefixes (not
// the cloud names) — the function maps cloud "scaleway" → "scaleway_",
// "gcp" → "google_", "aws" → "aws_", "genesys" → "genesyscloud_".
//
// We extract the resource prefix from each `return strings.HasPrefix(resource, "<prefix>_")`.
func readPrefixesFromSwitchCases(t *testing.T, path, funcName string) map[string]struct{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	// Locate the function. Slice from the function declaration to
	// the next top-level `^}` after it.
	funcDeclRe := regexp.MustCompile(`(?m)^func\s+` + regexp.QuoteMeta(funcName) + `\b`)
	loc := funcDeclRe.FindIndex(raw)
	if loc == nil {
		t.Fatalf("could not locate func %s in %s", funcName, path)
	}
	body := raw[loc[0]:]
	endRe := regexp.MustCompile(`(?m)^}\n`)
	endLoc := endRe.FindIndex(body)
	if endLoc == nil {
		t.Fatalf("could not locate end of func %s body in %s", funcName, path)
	}
	body = body[:endLoc[0]]

	hasPrefixRe := regexp.MustCompile(`strings\.HasPrefix\(\s*resource\s*,\s*"([a-z0-9]+)_"\s*\)`)
	matches := hasPrefixRe.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		t.Fatalf("no strings.HasPrefix(resource, ...) calls found in %s — has the function shape changed?", funcName)
	}

	out := map[string]struct{}{}
	for _, m := range matches {
		out[string(m[1])] = struct{}{}
	}
	return out
}

// repoRoot walks up from this file's directory until it finds a go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root from %s", file)
	return ""
}

// setEqual reports whether two prefix sets contain the same elements.
func setEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// diff returns elements in a not in b, sorted for stable output.
func diff(a, b map[string]struct{}) []string {
	out := make([]string, 0)
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// sorted returns the keys of m in lexicographic order.
func sorted(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
