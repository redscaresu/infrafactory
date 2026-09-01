// pitfall-merge — selectively preserve sweep pitfall additions.
//
// Reads two pitfall YAML files (pre-sweep + post-sweep) and writes a
// merged YAML that is pre-sweep PLUS any post-sweep entries whose
// `source` matches one of the --keep values. Entries are deduped by
// (resource, rule) — if the same entry already exists in pre, the
// post copy is skipped.
//
// Used by scripts/sweep_39.sh to replace the blanket
// `git checkout pitfalls/` with selective restoration that keeps
// N13's `avoid` durably while still discarding
// `descriptive` + `fix` as sweep noise.
//
// Rationale: N13 only fires when iter N+1 cleared a failure by
// deleting a resource — the output is grounded in a confirmed
// successful run, not a guess. The other two sources are more
// speculative and stay discardable.
//
// Usage:
//
//	bin/pitfall-merge --pre /tmp/pre/aws.yaml --post pitfalls/aws.yaml \
//	  --out pitfalls/aws.yaml --keep avoid
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/generator"
	"gopkg.in/yaml.v3"
)

func main() {
	preFile := flag.String("pre", "", "pre-sweep pitfalls yaml path (required)")
	postFile := flag.String("post", "", "post-sweep pitfalls yaml path (required)")
	outFile := flag.String("out", "", "output merged yaml path (required)")
	// `live` joins `avoid` in the default: both are run-derived, and a
	// sweep that discards live entries would delete learning SILENTLY --
	// which is the one thing S156a's retirement path exists to prevent.
	// Retirement names what it removes; a sweep dropping the same entries
	// on the floor would make the corpus untrustworthy in exactly the way
	// the reporting was meant to fix.
	keepFlag := flag.String("keep", "avoid,live", "comma-separated source values to preserve from post")
	flag.Parse()

	if *preFile == "" || *postFile == "" || *outFile == "" {
		fmt.Fprintln(os.Stderr, "usage: pitfall-merge --pre PRE --post POST --out OUT [--keep SOURCES]")
		os.Exit(2)
	}

	keepSet := map[string]bool{}
	for _, s := range strings.Split(*keepFlag, ",") {
		if s = strings.TrimSpace(s); s != "" {
			keepSet[s] = true
		}
	}

	pre, err := loadPitfalls(*preFile)
	if err != nil {
		die("read pre: %v", err)
	}
	post, err := loadPitfalls(*postFile)
	if err != nil {
		die("read post: %v", err)
	}

	merged, added, refreshed := merge(pre, post, keepSet)

	if err := savePitfalls(*outFile, merged); err != nil {
		die("write out: %v", err)
	}

	fmt.Printf("pitfall-merge: pre=%d post=%d kept_new=%d refreshed=%d (sources: %s)\n",
		len(pre.Pitfalls), len(post.Pitfalls), added, refreshed, strings.Join(sortedKeys(keepSet), ","))
}

// merge returns pre + any post entries whose source is in keepSet and
// whose (resource, rule) is not already in pre, and carries forward a
// newer last_seen for the ones that are. Returns the counts of
// preserved-new and refreshed entries.
//
// The refresh half matters as much as the append half. A live entry
// re-observed during the sweep exists in BOTH files, so skipping it as a
// duplicate keeps pre's older timestamp -- which makes retention mean
// "first observed" instead of "last observed", exactly what
// TouchLivePitfall exists to prevent. The entry then retires early, and
// a rule that is still true is deleted.
func merge(pre, post generator.PitfallsFile, keepSet map[string]bool) (generator.PitfallsFile, int, int) {
	preIdx := map[string]int{}
	for i, p := range pre.Pitfalls {
		preIdx[entryKey(p)] = i
	}

	out := pre
	added, refreshed := 0, 0
	for _, p := range post.Pitfalls {
		if !keepSet[p.Source] {
			continue
		}
		if i, seen := preIdx[entryKey(p)]; seen {
			// Only refresh LIKE with LIKE. entryKey is (resource, rule)
			// and deliberately ignores source, so the same rule can
			// exist as `descriptive` in pre and `live` in post --
			// and copying the live timestamp onto the descriptive entry
			// would attach a lifetime to something that has none, while
			// the live record disappeared as a duplicate. Two errors from
			// one line.
			//
			// A source mismatch keeps the pre-existing behaviour: skip it,
			// exactly as this merge did before timestamps existed.
			if out.Pitfalls[i].Source == p.Source && newerLastSeen(out.Pitfalls[i].LastSeen, p.LastSeen) {
				out.Pitfalls[i].LastSeen = p.LastSeen
				refreshed++
			}
			continue
		}
		out.Pitfalls = append(out.Pitfalls, p)
		preIdx[entryKey(p)] = len(out.Pitfalls) - 1
		added++
	}
	return out, added, refreshed
}

// newerLastSeen reports whether candidate is a later timestamp than
// current.
//
// An unparseable or absent candidate never wins: the existing value is
// evidence somebody recorded, and replacing it with something unreadable
// would make the entry look never-seen and retire it.
func newerLastSeen(current, candidate string) bool {
	next, err := time.Parse(time.RFC3339, candidate)
	if err != nil {
		return false
	}
	prev, err := time.Parse(time.RFC3339, current)
	if err != nil {
		// Current is missing or malformed and candidate is valid, so
		// candidate is strictly more information.
		return true
	}
	return next.After(prev)
}

func entryKey(p generator.PitfallEntry) string {
	return p.Resource + "\x00" + p.Rule
}

func loadPitfalls(path string) (generator.PitfallsFile, error) {
	var pf generator.PitfallsFile
	body, err := os.ReadFile(path)
	if err != nil {
		return pf, err
	}
	if err := yaml.Unmarshal(body, &pf); err != nil {
		return pf, err
	}
	return pf, nil
}

func savePitfalls(path string, pf generator.PitfallsFile) error {
	body, err := yaml.Marshal(pf)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Tiny set; bubble sort for stable output without importing sort.
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "pitfall-merge: "+format+"\n", args...)
	os.Exit(1)
}
