package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeLayer3Stack(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`
terraform {
  required_providers {
    scaleway = {
      source  = "scaleway/scaleway"
      version = "2.81.0"
    }
  }
}
`+body), 0o644))
	return dir
}

// The expression that stranded real infrastructure, verbatim.
//
// S164 (2026-09-07): `web-live-paris` indexed a private NIC's private_ips
// list, the list was empty at apply time, apply failed with "Invalid
// index" -- and then TEARDOWN FAILED WITH THE SAME ERROR, because
// `tofu destroy` evaluates the configuration rather than replaying
// state. `run_project_delete` could not rescue it either: Scaleway
// refuses to delete a project that still holds resources. Recovery meant
// hand-editing the workdir and running `tofu destroy` manually.
func TestLayer3RefusesAnIndexThatCouldStrandTheStack(t *testing.T) {
	dir := writeLayer3Stack(t, `
resource "scaleway_lb_backend" "main" {
  lb_id = scaleway_lb.main.id
  server_ips = [
    scaleway_instance_private_nic.web.private_ips[0].address,
  ]
}
`)

	err := validateLayer3HCLShape(dir, []string{"scaleway_lb*", "scaleway_instance*"})

	require.Error(t, err, "this shape cost a stranded stack; it must not reach apply")
	assert.Contains(t, err.Error(), "private_ips")
	assert.Contains(t, err.Error(), "cannot be destroyed either",
		"the message must say why it is refused, not merely that it is")
	assert.Contains(t, err.Error(), "try() or one()",
		"and what to do instead")
}

// try() and one() make the expression total, so destroy can always
// evaluate it. That is the entire property being asked for.
func TestLayer3AcceptsAGuardedIndex(t *testing.T) {
	for name, body := range map[string]string{
		"try": `
resource "scaleway_lb_backend" "main" {
  lb_id = scaleway_lb.main.id
  server_ips = [try(scaleway_instance_private_nic.web.private_ips[0].address, "10.0.0.2")]
}`,
		"one": `
resource "scaleway_lb_backend" "main" {
  lb_id = scaleway_lb.main.id
  server_ips = [one(scaleway_instance_private_nic.web.private_ips)]
}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := validateLayer3HCLShape(writeLayer3Stack(t, body),
				[]string{"scaleway_lb*", "scaleway_instance*"})
			assert.NoError(t, err)
		})
	}
}

// Indexing something known before apply is not the hazard.
//
// A rule that refused `var.subnets[0]` would be refused by its users
// instead, and the property it protects -- that destroy can always
// evaluate the config -- is not at risk there.
func TestLayer3AllowsIndexingWhatIsKnownBeforeApply(t *testing.T) {
	err := validateLayer3HCLShape(writeLayer3Stack(t, `
variable "subnets" {
  type    = list(string)
  default = ["10.0.0.0/24"]
}

resource "scaleway_vpc_private_network" "main" {
  subnet = var.subnets[0]
}
`), []string{"scaleway_vpc*"})

	assert.NoError(t, err, "a variable is known at plan time and cannot strand a destroy")
}

// The guard must not leak past the call it belongs to.
func TestLayer3StillRefusesAnUnguardedIndexBesideAGuardedOne(t *testing.T) {
	err := validateLayer3HCLShape(writeLayer3Stack(t, `
resource "scaleway_lb_backend" "main" {
  lb_id = scaleway_lb.main.id
  safe   = try(scaleway_instance_private_nic.web.private_ips[0].address, "10.0.0.2")
  unsafe = scaleway_instance_server.web.public_ips[0].address
}
`), []string{"scaleway_lb*", "scaleway_instance*"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "public_ips", "the unguarded one is still caught")
	assert.NotContains(t, err.Error(), "private_ips", "the guarded one is not")
}

// The real stack, kept as a fixture.
//
// `output/web-live-paris` as it stood on 2026-09-07, when it created
// infrastructure that its own teardown could not remove. The two files
// this names -- loadbalancer.tf and outputs.tf -- are the two I had to
// hand-edit in the live workdir before `tofu destroy` would run.
//
// A synthetic snippet proves the matcher works. This proves it works on
// the thing that actually happened, which is a different claim: the
// stack has six files, nested blocks, variables and outputs, and the
// hazard is in two of them.
func TestTheStackThatStrandedIsRefused(t *testing.T) {
	err := validateLayer3HCLShape("testdata/undestroyable-stack", []string{
		"scaleway_lb*", "scaleway_instance*", "scaleway_vpc*",
	})

	require.Error(t, err, "this exact stack cost a manual recovery")
	assert.Contains(t, err.Error(), "loadbalancer.tf")
	assert.Contains(t, err.Error(), "outputs.tf",
		"both sites, not just the first: I had to patch both by hand")
	assert.Contains(t, err.Error(), "private_ips")
}
