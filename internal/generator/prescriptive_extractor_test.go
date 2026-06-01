package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractPrescriptiveFix_CMEKStorageBucket is the motivating
// case for N10. The 2026-06-02 sweep showed gcp-storage learn the
// symptom "missing encryption.default_kms_key_name" but never
// converge because nothing prescribed the fix shape. With the
// extractor, after one successful run the diff yields a snippet
// that the next iteration can lift verbatim.
func TestExtractPrescriptiveFix_CMEKStorageBucket(t *testing.T) {
	failedDir := t.TempDir()
	passingDir := t.TempDir()

	writeTF(t, failedDir, "storage.tf", `
resource "google_storage_bucket" "app_assets" {
  name                        = "app-assets-bucket"
  location                    = "us-central1"
  force_destroy               = true
  uniform_bucket_level_access = true
}
`)

	writeTF(t, passingDir, "storage.tf", `
resource "google_storage_bucket" "app_assets" {
  name                        = "app-assets-bucket"
  location                    = "us-central1"
  force_destroy               = true
  uniform_bucket_level_access = true

  encryption {
    default_kms_key_name = google_kms_crypto_key.app_assets.id
  }
}
`)
	writeTF(t, passingDir, "kms.tf", `
resource "google_kms_key_ring" "app_assets" {
  name     = "app-assets-keyring"
  location = "us-central1"
}

resource "google_kms_crypto_key" "app_assets" {
  name     = "app-assets-key"
  key_ring = google_kms_key_ring.app_assets.id
}
`)

	failureDetail := `google_storage_bucket.app_assets has no encryption.default_kms_key_name — customer-managed encryption not configured`
	entry, err := ExtractPrescriptiveFix(failedDir, passingDir, failureDetail, "google_storage_bucket.app_assets", "gcp", "gcp-storage", "20260602T100000Z")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry == nil {
		t.Fatal("expected a PitfallEntry, got nil")
	}
	if entry.Resource != "google_storage_bucket" {
		t.Errorf("resource = %q, want google_storage_bucket", entry.Resource)
	}
	if entry.Source != PrescriptiveSource {
		t.Errorf("source = %q, want %q", entry.Source, PrescriptiveSource)
	}
	if !strings.Contains(entry.Rule, "encryption") {
		t.Errorf("rule missing 'encryption': %q", entry.Rule)
	}
	if !strings.Contains(entry.Rule, "google_kms_crypto_key") {
		t.Errorf("rule missing companion kms resource: %q", entry.Rule)
	}
	if !strings.Contains(entry.Rule, "default_kms_key_name") {
		t.Errorf("rule missing prescriptive attribute: %q", entry.Rule)
	}
}

// TestExtractPrescriptiveFix_NoChangeReturnsNil exercises the
// conservative-by-design guard: if the failing resource's body is
// identical between iterations, we have no evidence the LLM
// changed anything relevant, so no pitfall is written.
func TestExtractPrescriptiveFix_NoChangeReturnsNil(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	body := `
resource "google_storage_bucket" "app" {
  name     = "app"
  location = "us-central1"
}
`
	writeTF(t, d1, "main.tf", body)
	writeTF(t, d2, "main.tf", body)

	entry, err := ExtractPrescriptiveFix(d1, d2, "google_storage_bucket.app has no encryption", "google_storage_bucket.app", "gcp", "scenario", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil, got %+v", entry)
	}
}

// TestExtractPrescriptiveFix_WhitespaceOnlyChangeIgnored guards the
// normaliser: cosmetic re-formatting (alignment, blank lines) must
// not register as a diff.
func TestExtractPrescriptiveFix_WhitespaceOnlyChangeIgnored(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `
resource "google_storage_bucket" "app" {
  name     = "app"
  location = "us-central1"
}
`)
	writeTF(t, d2, "main.tf", `
resource "google_storage_bucket" "app" {
  name      =    "app"

  location = "us-central1"
}
`)
	entry, err := ExtractPrescriptiveFix(d1, d2, "google_storage_bucket.app fail", "google_storage_bucket.app", "gcp", "scenario", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil (whitespace-only), got %+v", entry)
	}
}

// TestExtractPrescriptiveFix_FailureWithoutAddress is the fall-through
// case: the detail doesn't contain a `TYPE.NAME` reference, so the
// extractor can't attribute the diff to a specific resource.
func TestExtractPrescriptiveFix_FailureWithoutAddress(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `resource "google_storage_bucket" "app" { name = "a" }`)
	writeTF(t, d2, "main.tf", `resource "google_storage_bucket" "app" { name = "a" force_destroy = true }`)
	entry, err := ExtractPrescriptiveFix(d1, d2, "orphan_check detected 1 orphaned resources", "", "gcp", "scenario", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil (no address), got %+v", entry)
	}
}

// TestExtractPrescriptiveFix_StatePolicyDetailFallsBackToTypeHint
// pins the N14 attribution fallback. State-side policy failures emit
// details like "Cloud SQL instance NAME missing
// diskEncryptionConfiguration.kmsKeyName" — no terraform address.
// Without the fallback, the extractor returned nil and gcp-cloud-sql's
// learned_from_diff entry never landed. With the fallback, the
// extractor maps "Cloud SQL instance" → google_sql_database_instance
// and finds the one changed instance in the passing dir.
func TestExtractPrescriptiveFix_StatePolicyDetailFallsBackToTypeHint(t *testing.T) {
	failedDir := t.TempDir()
	passingDir := t.TempDir()

	writeTF(t, failedDir, "db.tf", `
resource "google_sql_database_instance" "postgres" {
  name             = "infrafactory-pg-run1"
  database_version = "POSTGRES_14"
  region           = "europe-west1"
  settings {
    tier = "db-f1-micro"
  }
}
`)

	writeTF(t, passingDir, "db.tf", `
resource "google_sql_database_instance" "postgres" {
  name                = "infrafactory-pg-run1"
  database_version    = "POSTGRES_14"
  region              = "europe-west1"
  encryption_key_name = google_kms_crypto_key.sql.id
  settings {
    tier = "db-f1-micro"
  }
}
`)
	writeTF(t, passingDir, "kms.tf", `
resource "google_kms_key_ring" "sql" {
  name     = "sql-ring"
  location = "europe-west1"
}

resource "google_kms_crypto_key" "sql" {
  name     = "sql-key"
  key_ring = google_kms_key_ring.sql.id
}
`)

	failureDetail := "Cloud SQL instance infrafactory-pg-run1 missing diskEncryptionConfiguration.kmsKeyName"
	entry, err := ExtractPrescriptiveFix(failedDir, passingDir, failureDetail, "", "gcp", "gcp-cloud-sql", "20260602T210000Z")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry from type-hint fallback, got nil")
	}
	if entry.Resource != "google_sql_database_instance" {
		t.Errorf("resource = %q, want google_sql_database_instance", entry.Resource)
	}
	if !strings.Contains(entry.Rule, "encryption_key_name") {
		t.Errorf("rule missing prescriptive attribute: %q", entry.Rule)
	}
	if !strings.Contains(entry.Rule, "google_kms_crypto_key") {
		t.Errorf("rule missing companion KMS sibling: %q", entry.Rule)
	}
}

// TestExtractPrescriptiveFix_TypeHintAmbiguousReturnsNil — when two
// resources of the inferred type changed, attribution is ambiguous
// and the extractor abstains rather than guess. Pins the "exactly
// one match" rule.
func TestExtractPrescriptiveFix_TypeHintAmbiguousReturnsNil(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `
resource "google_storage_bucket" "a" {
  name = "a"
}

resource "google_storage_bucket" "b" {
  name = "b"
}
`)
	writeTF(t, d2, "main.tf", `
resource "google_storage_bucket" "a" {
  name          = "a"
  force_destroy = true
}

resource "google_storage_bucket" "b" {
  name                        = "b"
  uniform_bucket_level_access = true
}
`)
	entry, err := ExtractPrescriptiveFix(d1, d2, "storage bucket a missing encryption.defaultKmsKeyName", "", "gcp", "s", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil (ambiguous), got %+v", entry)
	}
}

// TestExtractPrescriptiveFix_SnippetCap pins the 600-char cap. We
// generate a synthetic huge fix and assert the snippet ends with the
// truncation marker rather than mid-line.
func TestExtractPrescriptiveFix_SnippetCap(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `resource "google_storage_bucket" "app" { name = "a" }`)

	// Build a body of dozens of new attributes — well over 600 bytes.
	var b strings.Builder
	b.WriteString(`resource "google_storage_bucket" "app" {` + "\n")
	b.WriteString(`  name = "a"` + "\n")
	for i := 0; i < 100; i++ {
		b.WriteString("  label_" + repeat("x", 8) + " = \"value-" + repeat("y", 8) + "\"\n")
	}
	b.WriteString("}\n")
	writeTF(t, d2, "main.tf", b.String())

	entry, err := ExtractPrescriptiveFix(d1, d2, "google_storage_bucket.app fail", "google_storage_bucket.app", "gcp", "scenario", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry == nil {
		t.Fatal("expected an entry, got nil")
	}
	if len(entry.Rule) > snippetMaxBytes+400 {
		// Rule = summary + snippet; the snippet itself is capped at 600,
		// so total rule length should be bounded. Allow some room for
		// the leading summary text.
		t.Errorf("rule too long: %d bytes", len(entry.Rule))
	}
	if !strings.Contains(entry.Rule, "(truncated)") {
		t.Errorf("expected truncation marker in rule: %q", entry.Rule[:200])
	}
}

// TestExtractPrescriptiveFix_CrossCloudIsolation guards against
// learning a google_storage_bucket fix from a Scaleway scenario
// when no such resource exists. Should return nil.
func TestExtractPrescriptiveFix_CrossCloudIsolation(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `resource "scaleway_instance_server" "web" { name = "w" }`)
	writeTF(t, d2, "main.tf", `resource "scaleway_instance_server" "web" { name = "w" type = "DEV1-S" }`)

	entry, err := ExtractPrescriptiveFix(d1, d2, "google_storage_bucket.x has no encryption", "google_storage_bucket.x", "gcp", "scenario", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil (resource not in either dir), got %+v", entry)
	}
}

// --- helpers ---

func writeTF(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func repeat(s string, n int) string {
	return strings.Repeat(s, n)
}
