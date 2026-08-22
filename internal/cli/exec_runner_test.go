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

// TestStripGCPAuthEnvRemovesAllPrefixes guards the GCP credential-strip
// behavior added 2026-06-02. terraform-provider-google's v5 SDK probes
// the metadata server / ADC chain when any of these env vars are set,
// bypassing the access_token short-circuit and producing the misleading
// "ACCESS_TOKEN_TYPE_UNSUPPORTED" error against fakegcp. Stripping at the
// harness boundary guarantees the LLM's providers.tf credentials win.
func TestStripGCPAuthEnvRemovesAllPrefixes(t *testing.T) {
	t.Parallel()

	in := []string{
		"PATH=/usr/bin",
		"HOME=/Users/x",
		"GOOGLE_APPLICATION_CREDENTIALS=/var/keys/sa.json",
		"GOOGLE_CREDENTIALS=inline-json",
		"GOOGLE_CLOUD_KEYFILE_JSON=/etc/sa.json",
		"GOOGLE_OAUTH_ACCESS_TOKEN=ya29.xyz",
		"CLOUDSDK_CORE_PROJECT=my-proj",
		"CLOUDSDK_AUTH_ACCESS_TOKEN=abc",
		"GCLOUD_PROJECT=other-proj",
		"GOOGLE_REGION=us-central1", // not stripped — generic region setting
		"SCW_API_URL=http://mockway",
	}
	got := stripGCPAuthEnv(in)
	want := []string{
		"PATH=/usr/bin",
		"HOME=/Users/x",
		"GOOGLE_REGION=us-central1",
		"SCW_API_URL=http://mockway",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("strip mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

// TestStripEnvKeysExactAndWildcard covers the per-command strip added
// for Layer 3. Unlike stripGCPAuthEnv, which is unconditional, this one
// is opt-in per command because Layer 2 legitimately needs SCW_API_URL
// while Layer 3 must never see it.
func TestStripEnvKeysExactAndWildcard(t *testing.T) {
	t.Parallel()

	in := []string{
		"PATH=/usr/bin",
		"SCW_API_URL=http://localhost:8080",
		"SCW_ACCESS_KEY=keep-me",
		"SCW_DEFAULT_PROJECT_ID=00000000-0000-0000-0000-000000000000",
		"SCW_DEFAULT_REGION=fr-par",
		"MALFORMED_NO_EQUALS",
	}
	got := stripEnvKeys(in, []string{"SCW_API_URL", "SCW_DEFAULT_*"})
	want := []string{
		"PATH=/usr/bin",
		"SCW_ACCESS_KEY=keep-me",
		"MALFORMED_NO_EQUALS",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("strip mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestStripEnvKeysNoKeysIsIdentity(t *testing.T) {
	t.Parallel()

	in := []string{"A=1", "B=2"}
	if got := stripEnvKeys(in, nil); !slices.Equal(got, in) {
		t.Fatalf("expected identity, got %#v", got)
	}
}
