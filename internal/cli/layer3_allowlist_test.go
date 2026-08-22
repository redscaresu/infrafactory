package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
)

func writeTF(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

var canaryAllowlist = []string{
	"scaleway_account_project",
	"scaleway_block_volume",
	"scaleway_lb*",
}

func TestLayer3AllowlistPermitsExactAndPrefixMatches(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `
resource "scaleway_account_project" "run" {}
resource "scaleway_block_volume" "data" {}
resource "scaleway_lb_ip" "ip" {}
resource "scaleway_lb" "lb" {}
`)
	if err := validateLayer3ResourceAllowlist(dir, canaryAllowlist); err != nil {
		t.Fatalf("expected allowlisted types to pass, got: %v", err)
	}
}

// The failure this guard exists for: a repair iteration reaching for an
// expensive type. Against mockway that costs nothing; against real
// Scaleway it is minutes and money, repeated per iteration.
func TestLayer3AllowlistDeniesExpensiveTypes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `
resource "scaleway_account_project" "run" {}
resource "scaleway_k8s_cluster" "expensive" {}
resource "scaleway_rdb_instance" "also_expensive" {}
`)
	err := validateLayer3ResourceAllowlist(dir, canaryAllowlist)
	if err == nil {
		t.Fatal("expected denial of scaleway_k8s_cluster / scaleway_rdb_instance")
	}
	for _, want := range []string{"scaleway_k8s_cluster", "scaleway_rdb_instance"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name the denied type %q so the next iteration can route around it, got: %v", want, err)
		}
	}
	if !strings.Contains(err.Error(), "allow_resource_types") {
		t.Errorf("error should name the config key to widen, got: %v", err)
	}
}

// Deny-by-default. A config that forgot the list must stop the run, not
// provision whatever the LLM wrote.
func TestLayer3AllowlistEmptyDeniesEverything(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `resource "scaleway_block_volume" "data" {}`)

	for _, allowed := range [][]string{nil, {}} {
		if err := validateLayer3ResourceAllowlist(dir, allowed); err == nil {
			t.Fatalf("empty allowlist (%#v) must deny, not permit everything", allowed)
		}
	}
}

func TestLayer3AllowlistIgnoresNonTerraformFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTF(t, dir, "main.tf", `resource "scaleway_block_volume" "data" {}`)
	writeTF(t, dir, "notes.md", `resource "scaleway_k8s_cluster" "documented_but_not_declared" {}`)

	if err := validateLayer3ResourceAllowlist(dir, canaryAllowlist); err != nil {
		t.Fatalf("only .tf files should be scanned, got: %v", err)
	}
}

// The default shipped config must let the canary through and stop the
// expensive tier. Pins the actual default rather than a test fixture --
// a future edit that widens the default silently is what this catches.
func TestLayer3DefaultAllowlistCoversCanaryAndDeniesExpensive(t *testing.T) {
	t.Parallel()
	defaults := defaultSandboxAllowlistForTest(t)

	for _, allowed := range []string{"scaleway_account_project", "scaleway_block_volume", "scaleway_vpc"} {
		if !resourceTypeAllowed(allowed, defaults) {
			t.Errorf("default allowlist should permit %s (block-paris canary needs it)", allowed)
		}
	}
	for _, denied := range []string{"scaleway_k8s_cluster", "scaleway_rdb_instance", "scaleway_redis_cluster", "scaleway_instance_server"} {
		if resourceTypeAllowed(denied, defaults) {
			t.Errorf("default allowlist must NOT permit %s — it is slow and costly against real Scaleway", denied)
		}
	}
}

func defaultSandboxAllowlistForTest(t *testing.T) []string {
	t.Helper()
	return config.Default().Validation.Layers.SandboxDeploy.AllowResourceTypes
}
