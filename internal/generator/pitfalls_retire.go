package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LiveSource marks a pitfall learned from a service that was already
// running, rather than from a failed apply (S156, extending ADR-0019's
// vocabulary of descriptive / fix / avoid).
//
// It exists in this slice before anything produces one, because the
// retirement path has to be built before the inflow. Every other slice in
// S156 adds entries; this is the only thing that can take them out.
const LiveSource = "live"

// DefaultLiveRetention is how long a live-sourced pitfall keeps its place
// without being seen again.
//
// A guess, and labelled as one. There is no corpus of live entries yet to
// tune it against, so it is deliberately generous: retiring a rule that
// is still true costs a repeat of the failure it prevented, which is
// worse than carrying a stale rule for another fortnight. Raise or lower
// it from evidence once S156c produces real entries.
const DefaultLiveRetention = 14 * 24 * time.Hour

// RetiredPitfall is one entry that was removed, kept so the caller can
// report it.
type RetiredPitfall struct {
	Resource string
	Rule     string
	LastSeen time.Time
	Age      time.Duration
}

// RetireStaleLivePitfalls removes `source: live` entries that have not
// been seen within retention, and returns what it removed.
//
// Why live entries need this and the others do not: learning used to be
// bounded. A run emits at most `repair_iterations_max` failures, and a
// scenario that stops failing stops emitting. Live observation removed
// that bound -- a deployment running a broken image emits the same
// normalized failure on every probe, for its whole TTL, across every
// deployment of that scenario. The promotion gate limits the rate; only
// this removes an entry once the cause is fixed.
//
// And a stale pitfall is not inert. It steers generation away from
// something that is no longer broken, which makes every future
// generation worse **silently** -- the failure mode this project has
// learned to fear most.
//
// Three deliberate limits:
//
//   - Only `live` entries. Static, `fix` and `avoid` rules keep their
//     current unbounded lifetime; a rule extracted from a reproducible
//     apply failure does not stop being true because nobody hit it
//     lately.
//   - An entry with no LastSeen is NEVER retired. Absence of a timestamp
//     means nobody recorded when it was last true, which is not evidence
//     that it stopped being true. Guessing would delete rules on the
//     strength of a missing field.
//   - Removal is returned, never silent. Same rule as the D6 purge: a
//     corpus that quietly drops entries is indistinguishable from one
//     that never learned them.
func RetireStaleLivePitfalls(pitfallsDir, cloud string, retention time.Duration, now time.Time) ([]RetiredPitfall, error) {
	if err := assertRetention(retention); err != nil {
		return nil, err
	}

	pf, filePath, err := loadCloudPitfalls(pitfallsDir, cloud)
	if err != nil || pf == nil {
		return nil, err
	}

	kept, retired := partitionStale(pf.Pitfalls, retention, now)

	if len(retired) == 0 {
		// Nothing changed, so nothing is written. Rewriting an unchanged
		// file would churn its mtime and, on a corpus under review, make
		// "the retirement ran" look like "the retirement did something".
		return nil, nil
	}

	pf.Pitfalls = kept
	if err := writePitfallsFile(pitfallsDir, filePath, cloud, pf); err != nil {
		return nil, err
	}
	return retired, nil
}

// StaleLivePitfalls reports what RetireStaleLivePitfalls would remove,
// without removing it.
//
// Retirement deletes learning, so an operator should be able to see what
// a threshold would take out before it takes it out. Sharing
// partitionStale rather than reimplementing the rule is the point: a
// dry-run that can disagree with the real thing is worse than no dry-run.
func StaleLivePitfalls(pitfallsDir, cloud string, retention time.Duration, now time.Time) ([]RetiredPitfall, error) {
	if err := assertRetention(retention); err != nil {
		return nil, err
	}

	pf, _, err := loadCloudPitfalls(pitfallsDir, cloud)
	if err != nil || pf == nil {
		return nil, err
	}
	_, stale := partitionStale(pf.Pitfalls, retention, now)
	return stale, nil
}

// assertRetention refuses a window that would empty the corpus.
//
// Zero or negative makes every live entry infinitely stale, so a mistyped
// flag would delete everything learned from running services in one
// command. Checked on BOTH entry points, because a dry-run that accepts a
// value the real run rejects teaches an operator the wrong thing about
// what is safe to type.
func assertRetention(retention time.Duration) error {
	if retention <= 0 {
		return fmt.Errorf("retention must be positive, got %s: every live pitfall would be retired", retention)
	}
	return nil
}

// partitionStale splits entries into those that stay and those that go,
// newest-stale last so the report leads with the most overdue.
func partitionStale(entries []PitfallEntry, retention time.Duration, now time.Time) ([]PitfallEntry, []RetiredPitfall) {
	kept := make([]PitfallEntry, 0, len(entries))
	var stale []RetiredPitfall

	for _, entry := range entries {
		if !entry.retirable(now, retention) {
			kept = append(kept, entry)
			continue
		}
		lastSeen, _ := entry.lastSeenAt()
		stale = append(stale, RetiredPitfall{
			Resource: entry.Resource,
			Rule:     entry.Rule,
			LastSeen: lastSeen,
			Age:      now.Sub(lastSeen),
		})
	}

	sort.Slice(stale, func(i, j int) bool { return stale[i].Age > stale[j].Age })
	return kept, stale
}

// assertCloudName refuses anything that would escape the corpus when
// joined into a path.
//
// `cloud` arrives from the command line -- `pitfalls retire <cloud>` --
// and is joined straight onto the pitfalls directory. Without this,
// `retire ../../something` reads and REWRITES a YAML file outside the
// corpus entirely, which is a write, not just a read.
//
// The same guard livestore.validateID applies to deployment ids, for the
// same reason: a name that came from a caller decides a path.
func assertCloudName(cloud string) error {
	if strings.TrimSpace(cloud) == "" {
		return fmt.Errorf("cloud is required")
	}
	if cloud != strings.TrimSpace(cloud) {
		return fmt.Errorf("cloud %q has leading or trailing whitespace", cloud)
	}
	if strings.ContainsRune(cloud, os.PathSeparator) || strings.Contains(cloud, "/") {
		return fmt.Errorf("cloud %q contains a path separator", cloud)
	}
	if cloud == "." || cloud == ".." || strings.Contains(cloud, "..") {
		return fmt.Errorf("cloud %q contains a parent-directory reference", cloud)
	}
	return nil
}

// loadCloudPitfalls returns nil without error when the cloud has no
// corpus: nothing to retire is not a failure to retire.
//
// Named apart from the existing loadPitfallsFile(path), which returns
// only the entries -- retirement needs the whole file back so it can be
// written again.
func loadCloudPitfalls(pitfallsDir, cloud string) (*PitfallsFile, string, error) {
	if err := assertCloudName(cloud); err != nil {
		return nil, "", err
	}
	filePath := filepath.Join(pitfallsDir, cloud+".yaml")
	payload, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, filePath, nil
		}
		return nil, filePath, fmt.Errorf("read pitfalls for %s: %w", cloud, err)
	}

	var pf PitfallsFile
	if err := yaml.Unmarshal(payload, &pf); err != nil {
		return nil, filePath, fmt.Errorf("parse pitfalls for %s: %w", cloud, err)
	}
	return &pf, filePath, nil
}

// retirable reports whether this entry has outlived its usefulness.
func (e PitfallEntry) retirable(now time.Time, retention time.Duration) bool {
	if e.Source != LiveSource {
		return false
	}
	lastSeen, ok := e.lastSeenAt()
	if !ok {
		// No timestamp means nobody recorded when this was last true.
		// That is not evidence it stopped being true.
		return false
	}
	return now.Sub(lastSeen) > retention
}

// lastSeenAt parses the entry's timestamp, reporting whether it had a
// usable one. An unparseable value is treated as absent rather than as
// zero: zero would make every malformed entry infinitely stale and
// delete it.
func (e PitfallEntry) lastSeenAt() (time.Time, bool) {
	if e.LastSeen == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, e.LastSeen)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// TouchLivePitfall records that a live-sourced rule was seen again.
//
// Refreshing rather than appending is what makes retention mean "last
// observed" instead of "first observed". Without it, a rule that recurs
// every day for a month would still retire on the anniversary of the day
// it was learned.
func TouchLivePitfall(pitfallsDir, cloud, resource, rule string, now time.Time) error {
	if err := assertCloudName(cloud); err != nil {
		return err
	}
	filePath := filepath.Join(pitfallsDir, cloud+".yaml")
	payload, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read pitfalls for %s: %w", cloud, err)
	}

	var pf PitfallsFile
	if err := yaml.Unmarshal(payload, &pf); err != nil {
		return fmt.Errorf("parse pitfalls for %s: %w", cloud, err)
	}

	for i, entry := range pf.Pitfalls {
		if entry.Source != LiveSource || entry.Resource != resource || entry.Rule != rule {
			continue
		}
		pf.Pitfalls[i].LastSeen = now.UTC().Format(time.RFC3339)
		return writePitfallsFile(pitfallsDir, filePath, cloud, &pf)
	}

	return fmt.Errorf("no live pitfall for %s matching that rule", resource)
}

// AppendLivePitfall records a lesson learned from a running service,
// stamped so it can later be retired.
//
// The stamp is not optional. S156a never retires an entry with no
// `last_seen` -- absence means nobody recorded when the rule was last
// true, which is not evidence it stopped being true -- so a live entry
// written without one would be **immortal**, which is precisely what the
// retirement path exists to prevent. Writing the inflow without the
// timestamp would quietly undo the slice built to bound it.
//
// A rule seen again REFRESHES rather than duplicates, which is what makes
// retention mean "last observed" rather than "first observed".
func AppendLivePitfall(pitfallsDir, cloud, observedKey string, pitfall LearnedPitfall, now time.Time) error {
	pitfall.Source = LiveSource
	if observedKey == "" {
		return fmt.Errorf("a live pitfall needs an observed key, or it can never be recognised again")
	}

	// Already known: this is a re-observation, not a new lesson.
	//
	// Matched on the KEY rather than the rule text, because the text
	// states its evidence and that evidence grows. Refreshing updates the
	// text too, so the corpus carries the strongest evidence seen rather
	// than the first.
	if refreshed, err := refreshLivePitfall(pitfallsDir, cloud, observedKey, pitfall, now); err != nil {
		return err
	} else if refreshed {
		return nil
	}

	// Appended directly rather than through AppendPitfall, whose
	// deduplication is deliberately FUZZY -- it matches on significant
	// word overlap, which is right for provider diagnostics that vary in
	// phrasing between runs.
	//
	// Live rules are the opposite: generated from a template, so two
	// genuinely different failures on the same resource share nearly
	// every word ("Observed on a RUNNING deployment, after the apply
	// reported success: ... Evidence: ..."). Fuzzy matching would drop
	// the second as a duplicate of the first and the corpus would keep
	// whichever happened to be observed earliest, silently.
	//
	// Exact identity is also SOUND here in a way it is not for the fuzzy
	// path: the text is derived deterministically from the candidate, so
	// the same candidate always produces the same string, and a different
	// string means a different candidate.
	pf, filePath, err := loadCloudPitfalls(pitfallsDir, cloud)
	if err != nil {
		return err
	}
	if pf == nil {
		pf = &PitfallsFile{Provider: cloud}
	}

	pf.Pitfalls = append(pf.Pitfalls, PitfallEntry{
		Resource:       pitfall.Resource,
		Rule:           pitfall.Rule,
		Source:         LiveSource,
		DiscoveredFrom: pitfall.DiscoveredFrom,
		ObservedKey:    observedKey,
		LastSeen:       now.UTC().Format(time.RFC3339),
	})
	return writePitfallsFile(pitfallsDir, filePath, cloud, pf)
}

// refreshLivePitfall updates an existing live entry in place, reporting
// whether it found one.
//
// The rule text is rewritten as well as the timestamp: a candidate seen
// on more deployments is the same lesson with better evidence, and the
// corpus should carry the better version rather than whichever was
// written first.
func refreshLivePitfall(pitfallsDir, cloud, observedKey string, pitfall LearnedPitfall, now time.Time) (bool, error) {
	pf, filePath, err := loadCloudPitfalls(pitfallsDir, cloud)
	if err != nil || pf == nil {
		return false, err
	}

	for i, entry := range pf.Pitfalls {
		if entry.Source != LiveSource || entry.Resource != pitfall.Resource || entry.ObservedKey != observedKey {
			continue
		}
		pf.Pitfalls[i].Rule = pitfall.Rule
		pf.Pitfalls[i].DiscoveredFrom = pitfall.DiscoveredFrom
		pf.Pitfalls[i].LastSeen = now.UTC().Format(time.RFC3339)
		return true, writePitfallsFile(pitfallsDir, filePath, cloud, pf)
	}
	return false, nil
}
