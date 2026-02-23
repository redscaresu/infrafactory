package cli

import (
	"slices"
	"testing"
)

func TestWithEnvOverridesDeduplicatesOverriddenKeys(t *testing.T) {
	t.Parallel()

	base := []string{
		"SCW_API_URL=http://base",
		"PATH=/usr/bin",
		"SCW_API_URL=http://stale",
	}
	overrides := map[string]string{
		"SCW_API_URL": "http://override",
	}

	got := withEnvOverrides(base, overrides)
	want := []string{
		"PATH=/usr/bin",
		"SCW_API_URL=http://override",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected env output\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestWithEnvOverridesAppliesSortedOverridesAndPreservesOtherBaseEntries(t *testing.T) {
	t.Parallel()

	base := []string{
		"B=base",
		"A=base",
		"Z=base",
	}
	overrides := map[string]string{
		"A": "override-a",
		"C": "override-c",
	}

	got := withEnvOverrides(base, overrides)
	want := []string{
		"B=base",
		"Z=base",
		"A=override-a",
		"C=override-c",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected env output\nwant: %#v\ngot:  %#v", want, got)
	}
}
