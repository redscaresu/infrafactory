package main

import (
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

	got, added := merge(pre, post, map[string]bool{"avoid": true})

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

	got, added := merge(pre, post, map[string]bool{"avoid": true})

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

	got, added := merge(pre, post, map[string]bool{})

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

	got, added := merge(pre, post, map[string]bool{
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
