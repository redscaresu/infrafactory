package main

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/redscaresu/infrafactory/internal/generator"
)

func mk(resource, rule, source string) generator.PitfallEntry {
	return generator.PitfallEntry{Resource: resource, Rule: rule, Source: source, DiscoveredFrom: "test"}
}

// TestMerge_KeepsLearnedFromDiffAvoid pins the core S94 behaviour:
// post-sweep `avoid` additions land in the merged
// output; `descriptive` and `fix` additions are discarded.
func TestMerge_KeepsLearnedFromDiffAvoid(t *testing.T) {
	pre := generator.PitfallsFile{Provider: "gcp", Pitfalls: []generator.PitfallEntry{
		mk("google_compute_network", "existing rule body", "descriptive"),
	}}
	post := generator.PitfallsFile{Provider: "gcp", Pitfalls: []generator.PitfallEntry{
		mk("google_compute_network", "existing rule body", "descriptive"),             // unchanged
		mk("google_storage_bucket", "speculative N10 body", "fix"),                    // should drop
		mk("google_service_account", "do NOT use X", "avoid"),                         // should keep
		mk("google_sql_database_instance", "descriptive failure echo", "descriptive"), // should drop
	}}

	got, added, _ := merge(pre, post, map[string]bool{"avoid": true})

	if added != 1 {
		t.Fatalf("expected 1 added, got %d", added)
	}
	if len(got.Pitfalls) != 2 {
		t.Fatalf("expected 2 total in merged (1 pre + 1 N13), got %d", len(got.Pitfalls))
	}
	// Pre entry first (preserved at original position).
	if got.Pitfalls[0].Resource != "google_compute_network" {
		t.Errorf("pre entry not at position 0: %+v", got.Pitfalls[0])
	}
	// N13 entry appended.
	if got.Pitfalls[1].Source != "avoid" {
		t.Errorf("expected N13 entry at position 1, got: %+v", got.Pitfalls[1])
	}
	if got.Pitfalls[1].Resource != "google_service_account" {
		t.Errorf("expected google_service_account N13 entry, got: %+v", got.Pitfalls[1])
	}
}

// TestMerge_SkipsDuplicates pins dedup: an N13 entry in post that
// already exists in pre (same resource + rule) is NOT appended again.
func TestMerge_SkipsDuplicates(t *testing.T) {
	pre := generator.PitfallsFile{Provider: "gcp", Pitfalls: []generator.PitfallEntry{
		mk("google_service_account", "do NOT use X", "avoid"),
	}}
	post := generator.PitfallsFile{Provider: "gcp", Pitfalls: []generator.PitfallEntry{
		mk("google_service_account", "do NOT use X", "avoid"), // duplicate
	}}

	got, added, _ := merge(pre, post, map[string]bool{"avoid": true})

	if added != 0 {
		t.Errorf("expected 0 added (dup), got %d", added)
	}
	if len(got.Pitfalls) != 1 {
		t.Errorf("expected 1 total (no dup), got %d", len(got.Pitfalls))
	}
}

// TestMerge_EmptyKeepSet pins that with no sources to keep, the merge
// is equivalent to "restore pre-sweep" — no post additions land.
// This is the legacy `git checkout pitfalls/` behaviour, available as
// an opt-in via `--keep ""`.
func TestMerge_EmptyKeepSet(t *testing.T) {
	pre := generator.PitfallsFile{Provider: "gcp", Pitfalls: []generator.PitfallEntry{
		mk("a", "existing", "descriptive"),
	}}
	post := generator.PitfallsFile{Provider: "gcp", Pitfalls: []generator.PitfallEntry{
		mk("a", "existing", "descriptive"),
		mk("b", "new N13 entry", "avoid"),
	}}

	got, added, _ := merge(pre, post, map[string]bool{})

	if added != 0 {
		t.Errorf("empty keep-set: expected 0 added, got %d", added)
	}
	if len(got.Pitfalls) != 1 {
		t.Errorf("empty keep-set: expected merge to equal pre (1 entry), got %d", len(got.Pitfalls))
	}
}

// TestMerge_MultipleKeepSources pins the comma-separated --keep arg
// shape — caller could opt to keep `fix` too if a
// future arc trusts N10 output. Today, only `avoid`
// is in the default; the merge function itself supports any set.
func TestMerge_MultipleKeepSources(t *testing.T) {
	pre := generator.PitfallsFile{Provider: "gcp", Pitfalls: []generator.PitfallEntry{}}
	post := generator.PitfallsFile{Provider: "gcp", Pitfalls: []generator.PitfallEntry{
		mk("a", "N10 entry", "fix"),
		mk("b", "N13 entry", "avoid"),
		mk("c", "descriptive", "descriptive"),
	}}

	got, added, _ := merge(pre, post, map[string]bool{
		"fix":   true,
		"avoid": true,
	})

	if added != 2 {
		t.Errorf("expected 2 added (N10 + N13), got %d", added)
	}
	if len(got.Pitfalls) != 2 {
		t.Errorf("expected 2 total, got %d", len(got.Pitfalls))
	}
}

// A live entry re-observed during the sweep exists in BOTH files.
// Skipping it as a duplicate keeps pre's older timestamp, which makes
// retention mean "first observed" instead of "last observed" -- and the
// rule then retires early while it is still true.
func TestMergeCarriesForwardARefreshedLastSeen(t *testing.T) {
	const rule = "a live rule"
	pre := generator.PitfallsFile{Provider: "scaleway", Pitfalls: []generator.PitfallEntry{
		{Resource: "scaleway_lb", Rule: rule, Source: "live", LastSeen: "2026-08-01T00:00:00Z"},
	}}
	post := generator.PitfallsFile{Provider: "scaleway", Pitfalls: []generator.PitfallEntry{
		{Resource: "scaleway_lb", Rule: rule, Source: "live", LastSeen: "2026-09-01T00:00:00Z"},
	}}

	got, added, refreshed := merge(pre, post, map[string]bool{"live": true})

	assert.Zero(t, added, "it is the same entry, not a new one")
	assert.Equal(t, 1, refreshed)
	require.Len(t, got.Pitfalls, 1)
	assert.Equal(t, "2026-09-01T00:00:00Z", got.Pitfalls[0].LastSeen)
}

// Older or unreadable timestamps never win: the existing value is
// evidence somebody recorded, and replacing it with something unreadable
// would make the entry look never-seen and retire it.
func TestMergeKeepsTheBetterLastSeen(t *testing.T) {
	const rule = "a live rule"
	cases := map[string]struct{ pre, post, want string }{
		"older post":       {"2026-09-01T00:00:00Z", "2026-08-01T00:00:00Z", "2026-09-01T00:00:00Z"},
		"unparseable post": {"2026-09-01T00:00:00Z", "sometime", "2026-09-01T00:00:00Z"},
		"empty post":       {"2026-09-01T00:00:00Z", "", "2026-09-01T00:00:00Z"},
		"empty pre":        {"", "2026-09-01T00:00:00Z", "2026-09-01T00:00:00Z"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			pre := generator.PitfallsFile{Pitfalls: []generator.PitfallEntry{
				{Resource: "r", Rule: rule, Source: "live", LastSeen: tc.pre},
			}}
			post := generator.PitfallsFile{Pitfalls: []generator.PitfallEntry{
				{Resource: "r", Rule: rule, Source: "live", LastSeen: tc.post},
			}}

			got, _, _ := merge(pre, post, map[string]bool{"live": true})

			require.Len(t, got.Pitfalls, 1)
			assert.Equal(t, tc.want, got.Pitfalls[0].LastSeen)
		})
	}
}

// entryKey is (resource, rule) and ignores source, so the same rule can
// be `descriptive` in pre and `live` in post. Copying the live timestamp
// onto the descriptive entry would attach a lifetime to something that
// has none -- and only live entries are ever retired, so the timestamp
// would sit there meaning nothing while the live record vanished as a
// duplicate.
func TestMergeDoesNotRefreshAcrossSources(t *testing.T) {
	const rule = "the same rule, learned twice"
	pre := generator.PitfallsFile{Pitfalls: []generator.PitfallEntry{
		{Resource: "scaleway_lb", Rule: rule, Source: "descriptive"},
	}}
	post := generator.PitfallsFile{Pitfalls: []generator.PitfallEntry{
		{Resource: "scaleway_lb", Rule: rule, Source: "live", LastSeen: "2026-09-01T00:00:00Z"},
	}}

	got, added, refreshed := merge(pre, post, map[string]bool{"live": true})

	assert.Zero(t, refreshed, "a live timestamp must not land on a descriptive entry")
	assert.Zero(t, added, "and the pre-existing duplicate rule still wins, as it always did")
	require.Len(t, got.Pitfalls, 1)
	assert.Equal(t, "descriptive", got.Pitfalls[0].Source)
	assert.Empty(t, got.Pitfalls[0].LastSeen, "a source with no lifetime gains no timestamp")
}
