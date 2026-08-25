package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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

	// scaleway_instance_* is admitted so a load balancer can have a
	// backend that serves -- lb-serving-paris needs it (ADR-0023's
	// allowlist amendment). An instance boots in well under a minute,
	// unlike the clusters below.
	for _, allowed := range []string{
		"scaleway_account_project", "scaleway_block_volume", "scaleway_vpc",
		"scaleway_instance_ip", "scaleway_instance_server",
	} {
		if !resourceTypeAllowed(allowed, defaults) {
			t.Errorf("default allowlist should permit %s (the Layer 3 canaries need it)", allowed)
		}
	}
	for _, denied := range []string{"scaleway_k8s_cluster", "scaleway_rdb_instance", "scaleway_redis_cluster", "scaleway_instance_snapshot"} {
		if resourceTypeAllowed(denied, defaults) {
			t.Errorf("default allowlist must NOT permit %s — it is slow and costly against real Scaleway", denied)
		}
	}
}

// TestLayer3DefaultAllowlistMatchesCheckedInConfig keeps the two places
// that declare the allowlist honest.
//
// config.Default() applies when Layer 3 is enabled from a config that
// does not set allow_resource_types, and infrafactory.yaml applies
// otherwise. When they disagree, a scenario runs against real Scaleway
// from one entry point and is refused before the API from the other --
// with an error that points at the allowlist rather than at the drift.
func TestLayer3DefaultAllowlistMatchesCheckedInConfig(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("..", "..", "infrafactory.yaml"))
	require.NoError(t, err)

	var checkedIn struct {
		Validation struct {
			Layers struct {
				SandboxDeploy struct {
					AllowResourceTypes []string `yaml:"allow_resource_types"`
				} `yaml:"sandbox_deploy"`
			} `yaml:"layers"`
		} `yaml:"validation"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &checkedIn))

	want := checkedIn.Validation.Layers.SandboxDeploy.AllowResourceTypes
	require.NotEmpty(t, want, "infrafactory.yaml must declare an allowlist")
	assert.ElementsMatch(t, want, defaultSandboxAllowlistForTest(t),
		"internal/config/config.go and infrafactory.yaml must declare the same Layer 3 allowlist")
}

// Static policy and the allowlist must not contradict each other.
//
// policies/scaleway/vpc_required.rego DENIES any scaleway_instance_server
// that is not attached to a private network via
// scaleway_instance_private_nic. If the allowlist admits the server but
// not the NIC, then generated HCL that satisfies static policy is refused
// by the allowlist and HCL that satisfies the allowlist is refused by
// static policy -- no generated stack can pass both, and the error points
// at neither cause.
func TestLayer3AllowlistDoesNotContradictStaticPolicy(t *testing.T) {
	t.Parallel()
	defaults := defaultSandboxAllowlistForTest(t)

	if !resourceTypeAllowed("scaleway_instance_server", defaults) {
		t.Skip("instance servers are not allowlisted, so the policy coupling does not apply")
	}
	assert.True(t, resourceTypeAllowed("scaleway_instance_private_nic", defaults),
		"vpc_required.rego requires a private NIC on every instance server, so admitting servers "+
			"without admitting NICs makes the two gates unsatisfiable together")
}

func defaultSandboxAllowlistForTest(t *testing.T) []string {
	t.Helper()
	return config.Default().Validation.Layers.SandboxDeploy.AllowResourceTypes
}
