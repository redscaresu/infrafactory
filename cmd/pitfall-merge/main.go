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
// N13's `learned_from_diff_avoid` durably while still discarding
// `learned` + `learned_from_diff` as sweep noise.
//
// Rationale: N13 only fires when iter N+1 cleared a failure by
// deleting a resource — the output is grounded in a confirmed
// successful run, not a guess. The other two sources are more
// speculative and stay discardable.
//
// Usage:
//
//	bin/pitfall-merge --pre /tmp/pre/aws.yaml --post pitfalls/aws.yaml \
//	  --out pitfalls/aws.yaml --keep learned_from_diff_avoid
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/redscaresu/infrafactory/internal/generator"
	"gopkg.in/yaml.v3"
)

func main() {
	preFile := flag.String("pre", "", "pre-sweep pitfalls yaml path (required)")
	postFile := flag.String("post", "", "post-sweep pitfalls yaml path (required)")
	outFile := flag.String("out", "", "output merged yaml path (required)")
	keepFlag := flag.String("keep", "learned_from_diff_avoid", "comma-separated source values to preserve from post")
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

	merged, added := merge(pre, post, keepSet)

	if err := savePitfalls(*outFile, merged); err != nil {
		die("write out: %v", err)
	}

	fmt.Printf("pitfall-merge: pre=%d post=%d kept_new=%d (sources: %s)\n",
		len(pre.Pitfalls), len(post.Pitfalls), added, strings.Join(sortedKeys(keepSet), ","))
}

// merge returns pre + any post entries whose source is in keepSet and
// whose (resource, rule) is not already in pre. Returns the count of
// preserved-new entries.
func merge(pre, post generator.PitfallsFile, keepSet map[string]bool) (generator.PitfallsFile, int) {
	preKeys := map[string]bool{}
	for _, p := range pre.Pitfalls {
		preKeys[entryKey(p)] = true
	}

	out := pre
	added := 0
	for _, p := range post.Pitfalls {
		if !keepSet[p.Source] {
			continue
		}
		if preKeys[entryKey(p)] {
			continue
		}
		out.Pitfalls = append(out.Pitfalls, p)
		preKeys[entryKey(p)] = true
		added++
	}
	return out, added
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
