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

	merged, added, refreshed, addedBySource := merge(pre, post, keepSet)

	if err := savePitfalls(*outFile, merged); err != nil {
		die("write out: %v", err)
	}

	// Per-source counts as well as the total. AVOID_EMISSIONS in
	// scripts/sweep_39.sh is a ratchet on whether the avoid extractor
	// still works, and a combined kept_new would let preserved `live`
	// entries mask an avoid-learning regression -- a metric that reads
	// healthy while the thing it measures is broken.
	fmt.Printf("pitfall-merge: pre=%d post=%d kept_new=%d refreshed=%d %s(sources: %s)\n",
		len(pre.Pitfalls), len(post.Pitfalls), added, refreshed,
		perSourceCounts(addedBySource), strings.Join(sortedKeys(keepSet), ","))
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
func merge(pre, post generator.PitfallsFile, keepSet map[string]bool) (generator.PitfallsFile, int, int, map[string]int) {
	preIdx := map[string]int{}
	for i, p := range pre.Pitfalls {
		preIdx[mergeKey(p)] = i
	}

	out := pre
	added, refreshed := 0, 0
	addedBySource := map[string]int{}
	for _, p := range post.Pitfalls {
		if !keepSet[p.Source] {
			continue
		}
		if i, seen := preIdx[mergeKey(p)]; seen {
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
				// The text travels with the timestamp for live entries:
				// a later observation of the same failure carries better
				// evidence, and keeping the older wording would preserve
				// the weaker claim.
				if p.Source == generator.LiveSource {
					out.Pitfalls[i].Rule = p.Rule
					out.Pitfalls[i].DiscoveredFrom = p.DiscoveredFrom
				}
				refreshed++
			}
			continue
		}
		out.Pitfalls = append(out.Pitfalls, p)
		preIdx[mergeKey(p)] = len(out.Pitfalls) - 1
		added++
		addedBySource[p.Source]++
	}
	return out, added, refreshed, addedBySource
}

// perSourceCounts renders `kept_avoid=2 kept_live=1 `, or nothing when a
// merge preserved nothing, so a caller can ratchet on one source without
// parsing a combined total.
func perSourceCounts(bySource map[string]int) string {
	if len(bySource) == 0 {
		return ""
	}
	var b strings.Builder
	for _, src := range sortedKeys(toBoolSet(bySource)) {
		fmt.Fprintf(&b, "kept_%s=%d ", src, bySource[src])
	}
	return b.String()
}

func toBoolSet(counts map[string]int) map[string]bool {
	set := make(map[string]bool, len(counts))
	for k := range counts {
		set[k] = true
	}
	return set
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

// mergeKey adds the source for entries that carry a LIFETIME.
//
// `live` is the only source with state that cannot be reconstructed: its
// last_seen is the whole basis of retirement. Keying it on (resource,
// rule) alone means an older `descriptive` entry with the same text
// swallows it as a duplicate, and the observation -- along with the
// timestamp that decides when it retires -- is silently gone.
//
// Every other source keeps the historical (resource, rule) identity.
// Dropping a duplicate `avoid` loses a rule the corpus already states in
// other words; dropping a duplicate `live` loses information nothing can
// rebuild. That asymmetry is the reason for the special case, and the
// reason it is not applied to everything.
func mergeKey(p generator.PitfallEntry) string {
	if p.Source != generator.LiveSource {
		return entryKey(p)
	}
	// Live entries are identified by their OBSERVED KEY, not their rule
	// text. The text states its evidence -- how many deployments, how
	// long a run -- and that evidence grows as the same failure keeps
	// being observed, so keying on the text would make one lesson look
	// like a new one on every sweep and the corpus would gain a copy per
	// merge (S156c, pass 77).
	//
	// Falls back to the text for entries written before observed_key
	// existed: no key is worse than a stale one, but treating a
	// keyless entry as brand new every sweep would be worse still.
	identity := p.ObservedKey
	if identity == "" {
		identity = p.Rule
	}
	return p.Resource + "\x00" + identity + "\x00" + p.Source
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
