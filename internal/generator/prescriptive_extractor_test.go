package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractFixPitfall_CMEKStorageBucket is the motivating
// case for N10. The 2026-06-02 sweep showed gcp-storage learn the
// symptom "missing encryption.default_kms_key_name" but never
// converge because nothing prescribed the fix shape. With the
// extractor, after one successful run the diff yields a snippet
// that the next iteration can lift verbatim.
func TestExtractFixPitfall_CMEKStorageBucket(t *testing.T) {
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
	entry, err := ExtractFixPitfall(failedDir, passingDir, failureDetail, "google_storage_bucket.app_assets", "gcp", "gcp-storage", "20260602T100000Z")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry == nil {
		t.Fatal("expected a PitfallEntry, got nil")
	}
	if entry.Resource != "google_storage_bucket" {
		t.Errorf("resource = %q, want google_storage_bucket", entry.Resource)
	}
	if entry.Source != FixSource {
		t.Errorf("source = %q, want %q", entry.Source, FixSource)
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

// TestExtractFixPitfall_NoChangeReturnsNil exercises the
// conservative-by-design guard: if the failing resource's body is
// identical between iterations, we have no evidence the LLM
// changed anything relevant, so no pitfall is written.
func TestExtractFixPitfall_NoChangeReturnsNil(t *testing.T) {
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

	entry, err := ExtractFixPitfall(d1, d2, "google_storage_bucket.app has no encryption", "google_storage_bucket.app", "gcp", "scenario", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil, got %+v", entry)
	}
}

// TestExtractFixPitfall_WhitespaceOnlyChangeIgnored guards the
// normaliser: cosmetic re-formatting (alignment, blank lines) must
// not register as a diff.
func TestExtractFixPitfall_WhitespaceOnlyChangeIgnored(t *testing.T) {
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
	entry, err := ExtractFixPitfall(d1, d2, "google_storage_bucket.app fail", "google_storage_bucket.app", "gcp", "scenario", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil (whitespace-only), got %+v", entry)
	}
}

// TestExtractFixPitfall_FailureWithoutAddress is the fall-through
// case: the detail doesn't contain a `TYPE.NAME` reference, so the
// extractor can't attribute the diff to a specific resource.
func TestExtractFixPitfall_FailureWithoutAddress(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `resource "google_storage_bucket" "app" { name = "a" }`)
	writeTF(t, d2, "main.tf", `resource "google_storage_bucket" "app" { name = "a" force_destroy = true }`)
	entry, err := ExtractFixPitfall(d1, d2, "orphan_check detected 1 orphaned resources", "", "gcp", "scenario", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil (no address), got %+v", entry)
	}
}

// TestExtractFixPitfall_StatePolicyDetailFallsBackToTypeHint
// pins the N14 attribution fallback. State-side policy failures emit
// details like "Cloud SQL instance NAME missing
// diskEncryptionConfiguration.kmsKeyName" — no terraform address.
// Without the fallback, the extractor returned nil and gcp-cloud-sql's
// fix entry never landed. With the fallback, the
// extractor maps "Cloud SQL instance" → google_sql_database_instance
// and finds the one changed instance in the passing dir.
func TestExtractFixPitfall_StatePolicyDetailFallsBackToTypeHint(t *testing.T) {
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
	entry, err := ExtractFixPitfall(failedDir, passingDir, failureDetail, "", "gcp", "gcp-cloud-sql", "20260602T210000Z")
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

// TestExtractFixPitfall_TypeHintAmbiguousReturnsNil — when two
// resources of the inferred type changed, attribution is ambiguous
// and the extractor abstains rather than guess. Pins the "exactly
// one match" rule.
func TestExtractFixPitfall_TypeHintAmbiguousReturnsNil(t *testing.T) {
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
	entry, err := ExtractFixPitfall(d1, d2, "storage bucket a missing encryption.defaultKmsKeyName", "", "gcp", "s", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil (ambiguous), got %+v", entry)
	}
}

// TestExtractFixPitfall_SnippetCap pins the 600-char cap. We
// generate a synthetic huge fix and assert the snippet ends with the
// truncation marker rather than mid-line.
func TestExtractFixPitfall_SnippetCap(t *testing.T) {
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

	entry, err := ExtractFixPitfall(d1, d2, "google_storage_bucket.app fail", "google_storage_bucket.app", "gcp", "scenario", "ts")
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

// TestExtractFixPitfall_SnippetTrimAtBlockBoundary pins the
// trim improvement: when the snippet would exceed 600 bytes, prefer
// cutting after a top-level `}` so the example remains balanced HCL.
// Motivating case from the 2026-06-02 S55 audit: a
// google_sql_database_instance snippet's `depends_on = [` got cut
// mid-list inside a settings block, leaving the example unparseable.
func TestExtractFixPitfall_SnippetTrimAtBlockBoundary(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `resource "google_storage_bucket" "app" { name = "a" }`)

	// One realistic-sized block (~300 bytes) followed by an oversized
	// sibling — the trim should keep the first block intact and stop
	// at its `}` rather than slicing the second mid-block.
	var b strings.Builder
	b.WriteString(`resource "google_storage_bucket" "app" {` + "\n")
	b.WriteString(`  name     = "a"` + "\n")
	b.WriteString(`  location = "EU"` + "\n")
	b.WriteString(`  encryption {` + "\n")
	b.WriteString(`    default_kms_key_name = google_kms_crypto_key.k.id` + "\n")
	b.WriteString(`  }` + "\n")
	b.WriteString("}\n")
	b.WriteString(`resource "google_kms_crypto_key" "k" {` + "\n")
	for i := 0; i < 50; i++ {
		b.WriteString("  big_attr_" + repeat("x", 8) + " = \"" + repeat("y", 20) + "\"\n")
	}
	b.WriteString("}\n")
	writeTF(t, d2, "main.tf", b.String())

	entry, err := ExtractFixPitfall(d1, d2, "google_storage_bucket.app fail", "google_storage_bucket.app", "gcp", "scenario", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry == nil {
		t.Fatal("expected an entry, got nil")
	}
	if !strings.Contains(entry.Rule, "(truncated)") {
		t.Fatal("expected truncation marker")
	}
	// The portion before "(truncated)" must end with "}\n" — the
	// boundary cut. Anything else means we sliced mid-block.
	pre := entry.Rule[:strings.Index(entry.Rule, "# ... (truncated)")]
	if !strings.HasSuffix(pre, "}\n") {
		t.Errorf("expected snippet to end with `}\\n` before truncation marker; got tail %q", pre[max(0, len(pre)-40):])
	}
}

// TestExtractFixPitfall_CrossCloudIsolation guards against
// learning a google_storage_bucket fix from a Scaleway scenario
// when no such resource exists. Should return nil.
func TestExtractFixPitfall_CrossCloudIsolation(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `resource "scaleway_instance_server" "web" { name = "w" }`)
	writeTF(t, d2, "main.tf", `resource "scaleway_instance_server" "web" { name = "w" type = "DEV1-S" }`)

	entry, err := ExtractFixPitfall(d1, d2, "google_storage_bucket.x has no encryption", "google_storage_bucket.x", "gcp", "scenario", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil (resource not in either dir), got %+v", entry)
	}
}

// --- N13 deletion-as-fix tests ---

// TestExtractAvoidPitfall_AttributeRemoval is the motivating
// case for N13: the LLM clears an "Unsupported argument" failure by
// REMOVING the offending attribute. The avoid extractor should emit
// a "do NOT use" rule with the attribute name.
func TestExtractAvoidPitfall_AttributeRemoval(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `resource "google_cloud_run_v2_service" "api" {
  name              = "api"
  location          = "europe-west1"
  deletion_policy   = "DELETE"
}`)
	writeTF(t, d2, "main.tf", `resource "google_cloud_run_v2_service" "api" {
  name     = "api"
  location = "europe-west1"
}`)

	failureDetail := `exit status 1 | stderr: Error: Unsupported argument; An argument named "deletion_policy" is not expected here.`
	entry, err := ExtractAvoidPitfall(d1, d2, failureDetail, "google_cloud_run_v2_service.api", "gcp", "gcp-cloud-run", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry == nil {
		t.Fatal("expected an avoid entry, got nil")
	}
	if entry.Source != AvoidSource {
		t.Errorf("source = %q, want %q", entry.Source, AvoidSource)
	}
	if entry.Resource != "google_cloud_run_v2_service" {
		t.Errorf("resource = %q", entry.Resource)
	}
	if !strings.Contains(entry.Rule, "Do NOT use attribute `deletion_policy`") {
		t.Errorf("rule missing avoid clause: %q", entry.Rule)
	}
}

// TestExtractAvoidPitfall_ResourceRemoval covers case (b): the
// LLM clears the failure by dropping every resource of a given type.
// Motivating case: google_project_service removal to escape the v5
// provider's auth-pipeline preflight (the 2026-06-02 prompt-rule
// retirement that pre-dated N13).
func TestExtractAvoidPitfall_ResourceRemoval(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `resource "google_compute_instance" "vm" {
  name = "vm"
}
resource "google_project_service" "compute" {
  service = "compute.googleapis.com"
}
resource "google_project_service" "iam" {
  service = "iam.googleapis.com"
}
`)
	writeTF(t, d2, "main.tf", `resource "google_compute_instance" "vm" {
  name = "vm"
}
`)

	failureDetail := `exit status 1 | stderr: Error: ACCESS_TOKEN_TYPE_UNSUPPORTED on google_project_service.compute reaching cloudresourcemanager.googleapis.com`
	entry, err := ExtractAvoidPitfall(d1, d2, failureDetail, "google_project_service.compute", "gcp", "gcp-cloud-run", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry == nil {
		t.Fatal("expected an avoid entry, got nil")
	}
	if !strings.Contains(entry.Rule, "resource type `google_project_service`") {
		t.Errorf("rule missing resource-type avoid clause: %q", entry.Rule)
	}
}

// TestExtractAvoidPitfall_UnrelatedRemovalReturnsNil pins the
// attribution strictness: a removed attribute whose name does NOT
// appear in the failure detail (cosmetic LLM rewrite) MUST NOT
// produce an avoid entry — otherwise the file fills with noise.
func TestExtractAvoidPitfall_UnrelatedRemovalReturnsNil(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `resource "google_storage_bucket" "b" {
  name     = "x"
  location = "EU"
  labels   = { env = "test" }
}`)
	writeTF(t, d2, "main.tf", `resource "google_storage_bucket" "b" {
  name     = "x"
  location = "EU"
}`)

	failureDetail := `policy=gcp.encryption: google_storage_bucket.b has no encryption.default_kms_key_name`
	entry, err := ExtractAvoidPitfall(d1, d2, failureDetail, "google_storage_bucket.b", "gcp", "gcp-storage", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil (removed `labels` is unrelated to CMEK failure), got %+v", entry)
	}
}

// TestExtractAvoidPitfall_CamelCaseAttributeInFailureDetail
// pins the S63 audit fix. The aws_subnet `MapPublicIpOnLaunch` case
// surfaced as a false-positive in S63's sweep: N13 saw the failing
// iter remove `map_public_ip_on_launch` but couldn't attribute it
// because the AWS API error echoed the JSON-side `MapPublicIpOnLaunch`
// (camelCase) while the strict `strings.Contains(failureDetail, attr)`
// check only matched the snake_case form. `attributeAppearsInDetail`
// now tries case-insensitive + camelCase variants.
func TestExtractAvoidPitfall_CamelCaseAttributeInFailureDetail(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `resource "aws_subnet" "public" {
  cidr_block                = "10.0.1.0/24"
  availability_zone         = "us-east-1a"
  map_public_ip_on_launch   = true
}`)
	writeTF(t, d2, "main.tf", `resource "aws_subnet" "public" {
  cidr_block        = "10.0.1.0/24"
  availability_zone = "us-east-1a"
}`)

	// Real AWS error shape (camelCase echo of the offending field).
	failureDetail := `Error: waiting for EC2 Subnet (subnet-abc) MapPublicIpOnLaunch update: timeout while waiting for state to become 'true' (last state: 'false', timeout: 5m0s)`
	entry, err := ExtractAvoidPitfall(d1, d2, failureDetail, "aws_subnet.public", "aws", "aws-vpc-network", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry == nil {
		t.Fatal("expected an avoid entry attributing the snake_case attribute via the camelCase failure detail, got nil")
	}
	if !strings.Contains(entry.Rule, "attribute `map_public_ip_on_launch`") {
		t.Errorf("rule missing map_public_ip_on_launch avoid clause: %q", entry.Rule)
	}
}

// TestExtractAvoidPitfall_PartialResourceRemovalSkipped ensures
// case (b) only fires when ALL instances of a type are dropped. A
// scenario that goes from two `google_project_service` resources to
// one (the LLM kept compute, dropped iam) is too ambiguous — could
// be a legit narrowing — so skip the avoid emission.
func TestExtractAvoidPitfall_PartialResourceRemovalSkipped(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	writeTF(t, d1, "main.tf", `resource "google_compute_instance" "vm" {
  name = "vm"
}
resource "google_project_service" "compute" {
  service = "compute.googleapis.com"
}
resource "google_project_service" "iam" {
  service = "iam.googleapis.com"
}
`)
	writeTF(t, d2, "main.tf", `resource "google_compute_instance" "vm" {
  name = "vm"
}
resource "google_project_service" "compute" {
  service = "compute.googleapis.com"
}
`)

	failureDetail := `Error reaching google_project_service.iam (ACCESS_TOKEN_TYPE_UNSUPPORTED)`
	entry, err := ExtractAvoidPitfall(d1, d2, failureDetail, "google_project_service.iam", "gcp", "gcp-cloud-run", "ts")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if entry != nil && strings.Contains(entry.Rule, "resource type") {
		t.Errorf("expected no resource-type avoid (compute still present), got %q", entry.Rule)
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
