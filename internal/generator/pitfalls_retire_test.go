package generator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func writeCorpus(t *testing.T, entries ...PitfallEntry) string {
	t.Helper()
	dir := t.TempDir()
	payload, err := yaml.Marshal(PitfallsFile{Provider: "scaleway", Pitfalls: entries})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaleway.yaml"), payload, 0o644))
	return dir
}

func readCorpus(t *testing.T, dir string) []PitfallEntry {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(dir, "scaleway.yaml"))
	require.NoError(t, err)
	var pf PitfallsFile
	require.NoError(t, yaml.Unmarshal(payload, &pf))
	return pf.Pitfalls
}

func liveEntry(resource, rule string, lastSeen time.Time) PitfallEntry {
	e := PitfallEntry{Resource: resource, Rule: rule, Source: LiveSource}
	if !lastSeen.IsZero() {
		e.LastSeen = lastSeen.UTC().Format(time.RFC3339)
	}
	return e
}

func TestRetireRemovesLiveEntriesPastRetention(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	dir := writeCorpus(t,
		liveEntry("scaleway_lb", "stale rule", now.Add(-30*24*time.Hour)),
		liveEntry("scaleway_instance_server", "recent rule", now.Add(-1*time.Hour)),
	)

	retired, err := RetireStaleLivePitfalls(dir, "scaleway", 14*24*time.Hour, now)

	require.NoError(t, err)
	require.Len(t, retired, 1)
	assert.Equal(t, "scaleway_lb", retired[0].Resource)
	assert.Equal(t, 30*24*time.Hour, retired[0].Age)

	kept := readCorpus(t, dir)
	require.Len(t, kept, 1)
	assert.Equal(t, "scaleway_instance_server", kept[0].Resource)
}

// A rule extracted from a reproducible apply failure does not stop being
// true because nobody hit it lately. Only live entries decay.
func TestRetireLeavesEveryOtherSourceAlone(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	ancient := now.Add(-365 * 24 * time.Hour).UTC().Format(time.RFC3339)

	dir := writeCorpus(t,
		PitfallEntry{Resource: "a", Rule: "r", Source: DescriptiveSource, LastSeen: ancient},
		PitfallEntry{Resource: "b", Rule: "r", Source: FixSource, LastSeen: ancient},
		PitfallEntry{Resource: "c", Rule: "r", Source: AvoidSource, LastSeen: ancient},
		PitfallEntry{Resource: "d", Rule: "r", Source: "", LastSeen: ancient},
	)

	retired, err := RetireStaleLivePitfalls(dir, "scaleway", time.Hour, now)

	require.NoError(t, err)
	assert.Empty(t, retired)
	assert.Len(t, readCorpus(t, dir), 4)
}

// Absence of a timestamp means nobody recorded when the rule was last
// true. That is not evidence it stopped being true, and deleting on the
// strength of a missing field would lose learning for free.
func TestRetireNeverRemovesAnEntryWithNoTimestamp(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	dir := writeCorpus(t,
		liveEntry("scaleway_lb", "no timestamp", time.Time{}),
		PitfallEntry{Resource: "scaleway_rdb_instance", Rule: "unparseable", Source: LiveSource, LastSeen: "yesterday"},
	)

	retired, err := RetireStaleLivePitfalls(dir, "scaleway", time.Nanosecond, now)

	require.NoError(t, err)
	assert.Empty(t, retired, "a missing or malformed timestamp is unknown, not infinitely stale")
	assert.Len(t, readCorpus(t, dir), 2)
}

// Exactly at the boundary the rule is still within retention: retirement
// needs the age to EXCEED the window, so a threshold of 14d does not
// delete something last seen 14d ago to the second.
func TestRetireIsExclusiveAtTheBoundary(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	retention := 14 * 24 * time.Hour
	dir := writeCorpus(t, liveEntry("scaleway_lb", "exactly at the edge", now.Add(-retention)))

	retired, err := RetireStaleLivePitfalls(dir, "scaleway", retention, now)

	require.NoError(t, err)
	assert.Empty(t, retired)
}

// Nothing changed means nothing is written: rewriting an unchanged file
// would make "the retirement ran" look like "the retirement did
// something" to anyone watching mtimes.
func TestRetireDoesNotRewriteAnUnchangedCorpus(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	dir := writeCorpus(t, liveEntry("scaleway_lb", "fresh", now))
	path := filepath.Join(dir, "scaleway.yaml")

	before, err := os.Stat(path)
	require.NoError(t, err)

	retired, err := RetireStaleLivePitfalls(dir, "scaleway", time.Hour, now)
	require.NoError(t, err)
	require.Empty(t, retired)

	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime())
}

// A dry-run that can disagree with the real thing is worse than no
// dry-run, so both go through one rule.
func TestStaleLivePitfallsReportsWithoutChangingAnything(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	dir := writeCorpus(t, liveEntry("scaleway_lb", "stale", now.Add(-30*24*time.Hour)))

	stale, err := StaleLivePitfalls(dir, "scaleway", 14*24*time.Hour, now)
	require.NoError(t, err)
	require.Len(t, stale, 1)

	assert.Len(t, readCorpus(t, dir), 1, "reporting must not remove")

	retired, err := RetireStaleLivePitfalls(dir, "scaleway", 14*24*time.Hour, now)
	require.NoError(t, err)
	assert.Equal(t, stale, retired, "the dry-run and the real thing agree by construction")
}

// The most overdue first: an operator reading a long list wants the worst
// offender at the top.
func TestRetireReportsMostOverdueFirst(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	dir := writeCorpus(t,
		liveEntry("recent", "r", now.Add(-20*24*time.Hour)),
		liveEntry("ancient", "r", now.Add(-90*24*time.Hour)),
		liveEntry("middling", "r", now.Add(-40*24*time.Hour)),
	)

	retired, err := RetireStaleLivePitfalls(dir, "scaleway", 14*24*time.Hour, now)

	require.NoError(t, err)
	require.Len(t, retired, 3)
	assert.Equal(t, "ancient", retired[0].Resource)
	assert.Equal(t, "middling", retired[1].Resource)
	assert.Equal(t, "recent", retired[2].Resource)
}

func TestRetireOnACloudWithNoCorpusIsNotAnError(t *testing.T) {
	retired, err := RetireStaleLivePitfalls(t.TempDir(), "aws", time.Hour, time.Now())

	assert.NoError(t, err, "nothing to retire is not a failure to retire")
	assert.Empty(t, retired)
}

func TestRetireRefusesANonPositiveRetention(t *testing.T) {
	dir := writeCorpus(t, liveEntry("scaleway_lb", "r", time.Now()))

	for _, retention := range []time.Duration{0, -time.Hour} {
		_, err := RetireStaleLivePitfalls(dir, "scaleway", retention, time.Now())
		assert.Error(t, err, "retention %s would retire everything ever learned", retention)
	}
}

// Refreshing rather than appending is what makes retention mean "last
// observed" instead of "first observed". Without it a rule that recurs
// daily for a month still retires on the anniversary of the day it was
// learned.
func TestTouchRefreshesLastSeenSoARecurringRuleSurvives(t *testing.T) {
	learned := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	dir := writeCorpus(t, liveEntry("scaleway_lb", "the rule", learned))

	require.NoError(t, TouchLivePitfall(dir, "scaleway", "scaleway_lb", "the rule", now))

	retired, err := RetireStaleLivePitfalls(dir, "scaleway", 14*24*time.Hour, now)
	require.NoError(t, err)
	assert.Empty(t, retired, "seen today, so not stale despite being learned a month ago")
}

func TestTouchReportsWhenThereIsNoSuchLivePitfall(t *testing.T) {
	dir := writeCorpus(t,
		PitfallEntry{Resource: "scaleway_lb", Rule: "a static rule", Source: DescriptiveSource})

	// Right resource, wrong source: only live entries carry LastSeen.
	err := TouchLivePitfall(dir, "scaleway", "scaleway_lb", "a static rule", time.Now())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no live pitfall")
}

// `cloud` arrives from the command line and is joined straight onto the
// pitfalls directory, so without a guard `retire ../../x` REWRITES a file
// outside the corpus. The same guard livestore.validateID applies to
// deployment ids, for the same reason.
func TestRetireRefusesACloudNameThatEscapesTheCorpus(t *testing.T) {
	dir := writeCorpus(t, liveEntry("scaleway_lb", "r", time.Now()))

	for _, cloud := range []string{
		"../escape", "sub/dir", "..", ".", " scaleway", "scaleway ", "",
		"a/../../b",
	} {
		_, err := RetireStaleLivePitfalls(dir, cloud, time.Hour, time.Now())
		assert.Error(t, err, "cloud %q must be refused", cloud)

		_, err = StaleLivePitfalls(dir, cloud, time.Hour, time.Now())
		assert.Error(t, err, "the dry-run must refuse it too: %q", cloud)

		err = TouchLivePitfall(dir, cloud, "r", "r", time.Now())
		assert.Error(t, err, "touch must refuse it too: %q", cloud)
	}
}

// AppendPitfall's deduplication is deliberately FUZZY: it matches on
// significant word overlap, which is right for provider diagnostics that
// vary in phrasing between runs.
//
// Live rules are the opposite — generated from a template, so two
// genuinely different failures on one resource share nearly every word.
// Routing them through the fuzzy path would keep whichever was observed
// first and discard the rest, silently.
func TestAppendLiveKeepsDistinctLessonsForOneResource(t *testing.T) {
	dir := writeCorpus(t)
	now := time.Now()

	const preamble = "Observed on a RUNNING deployment, after the apply reported success: "
	for _, detail := range []string{
		preamble + "health path returned HTTP 503. Evidence: persistent, across 1 deployment(s).",
		preamble + "health path is unreachable. Evidence: persistent, across 1 deployment(s).",
	} {
		require.NoError(t, AppendLivePitfall(dir, "scaleway", detail,
			LearnedPitfall{Resource: "scaleway_lb_ip", Rule: detail}, now))
	}

	entries := readCorpus(t, dir)
	require.Len(t, entries, 2, "two different failures are two lessons, however alike the wording")
	for _, e := range entries {
		assert.Equal(t, LiveSource, e.Source)
		assert.NotEmpty(t, e.LastSeen, "or it could never be retired")
	}
}

// The same lesson twice is one lesson, refreshed.
func TestAppendLiveRefreshesAnIdenticalLesson(t *testing.T) {
	dir := writeCorpus(t)
	learned := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	seen := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	p := LearnedPitfall{Resource: "scaleway_lb_ip", Rule: "the same words exactly"}

	require.NoError(t, AppendLivePitfall(dir, "scaleway", "the-key", p, learned))
	require.NoError(t, AppendLivePitfall(dir, "scaleway", "the-key", p, seen))

	entries := readCorpus(t, dir)
	require.Len(t, entries, 1)
	assert.Equal(t, seen.UTC().Format(time.RFC3339), entries[0].LastSeen,
		"retention means last observed, not first")
}

// A cloud with no corpus yet must gain one rather than fail: the first
// live lesson for a provider should not need a file to exist already.
func TestAppendLiveCreatesACorpusThatDoesNotExistYet(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, AppendLivePitfall(dir, "scaleway", "first-key",
		LearnedPitfall{Resource: "scaleway_lb_ip", Rule: "first lesson"}, time.Now()))

	entries := readCorpus(t, dir)
	require.Len(t, entries, 1)
	assert.Equal(t, "first lesson", entries[0].Rule)
}

// A live rule states its evidence -- how many deployments, how long a
// run -- and that evidence GROWS as the same failure keeps being
// observed. Matching on the text would append a new entry every time the
// counters ticked, so the corpus would grow once per cron tick while
// looking like it was refreshing.
func TestAppendLiveRefreshesWhenTheEvidenceChangesButTheFailureDoesNot(t *testing.T) {
	dir := writeCorpus(t)
	learned := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	seen := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	const key = "health path returned HTTP 503"

	require.NoError(t, AppendLivePitfall(dir, "scaleway", key, LearnedPitfall{
		Resource: "scaleway_lb_ip",
		Rule:     "Observed: HTTP 503. Evidence: persistent, across 1 deployment(s), longest run 3.",
	}, learned))

	// Same failure, more evidence, so a different rule text.
	require.NoError(t, AppendLivePitfall(dir, "scaleway", key, LearnedPitfall{
		Resource: "scaleway_lb_ip",
		Rule:     "Observed: HTTP 503. Evidence: persistent, across 4 deployment(s), longest run 9.",
	}, seen))

	entries := readCorpus(t, dir)
	require.Len(t, entries, 1, "more evidence for one failure is not a second lesson")
	assert.Contains(t, entries[0].Rule, "across 4 deployment(s)",
		"and the corpus carries the stronger evidence, not the first written")
	assert.Equal(t, seen.UTC().Format(time.RFC3339), entries[0].LastSeen)
}

// Without a key an entry can never be recognised again, so it would
// duplicate on every pass. Refusing is better than writing something
// unmaintainable.
func TestAppendLiveRefusesWithoutAnObservedKey(t *testing.T) {
	dir := writeCorpus(t)

	err := AppendLivePitfall(dir, "scaleway", "",
		LearnedPitfall{Resource: "scaleway_lb_ip", Rule: "r"}, time.Now())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "never be recognised again")
	assert.Empty(t, readCorpus(t, dir))
}
