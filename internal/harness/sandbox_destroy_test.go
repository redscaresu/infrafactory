package harness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSandboxDestroyHarnessRunSuccess(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{Stdout: []byte("destroy complete")}},
		},
	}

	h := NewSandboxDestroyHarness(runner)
	out, err := h.Run(context.Background(), "/tmp/workdir", map[string]string{"SCW_ACCESS_KEY": "real"})
	if err != nil {
		t.Fatalf("run sandbox destroy harness: %v", err)
	}
	if out.Destroy.Stage != "destroy" {
		t.Fatalf("unexpected result: %+v", out)
	}

	got := append([]string{runner.calls[0].Name}, runner.calls[0].Args...)
	expected := []string{"tofu", "destroy", "-auto-approve", "-state=" + LiveStateFilename}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("command mismatch: got %v want %v", got, expected)
		}
	}
}

func TestSandboxDestroyHarnessRunFailure(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{
		responses: []runnerResponse{
			{result: CommandResult{Stderr: []byte("destroy stderr")}, err: errors.New("destroy failed")},
		},
	}

	h := NewSandboxDestroyHarness(runner)
	_, err := h.Run(context.Background(), "/tmp/workdir", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrSandboxDestroyFailed) {
		t.Fatalf("expected ErrSandboxDestroyFailed, got %v", err)
	}

	var destroyErr *SandboxDestroyError
	if !errors.As(err, &destroyErr) {
		t.Fatalf("expected *SandboxDestroyError, got %T", err)
	}
	if destroyErr.Stage != "destroy" {
		t.Fatalf("expected destroy stage, got %q", destroyErr.Stage)
	}
}

const withoutConfigProject = "6c4390c9-664e-4289-a34f-cdc865653fc7"

// withoutConfigWorkDir writes a live state the way tofu does: each
// resource names its provider, and project-scoped ones their project.
func withoutConfigWorkDir(t *testing.T, resourceProject string, extra ...string) string {
	t.Helper()
	dir := t.TempDir()
	state := `{"resources": [` + strings.Join(append(extra, ""), ",") + `
	  {"mode": "managed", "type": "scaleway_vpc", "name": "main",
	   "provider": "provider[\"registry.opentofu.org/scaleway/scaleway\"]",
	   "instances": [{"attributes": {"id": "fr-par/vpc", "project_id": "` + resourceProject + `"}}]},
	  {"mode": "managed", "type": "scaleway_vpc", "name": "ams",
	   "provider": "provider[\"registry.opentofu.org/scaleway/scaleway\"].ams",
	   "instances": [{"attributes": {"id": "nl-ams/vpc", "project_id": "` + resourceProject + `"}}]},
	  {"mode": "managed", "type": "scaleway_vpc_private_network", "name": "main",
	   "provider": "provider[\"registry.opentofu.org/scaleway/scaleway\"]",
	   "instances": [{"attributes": {"id": "fr-par/pn", "project_id": "` + resourceProject + `"}}]},
	  {"mode": "managed", "type": "scaleway_instance_server", "name": "web",
	   "provider": "provider[\"registry.opentofu.org/scaleway/scaleway\"]",
	   "instances": [{"attributes": {"id": "fr-par-1/srv", "project_id": "` + resourceProject + `"}}]},
	  {"mode": "managed", "type": "scaleway_instance_private_nic", "name": "web",
	   "provider": "provider[\"registry.opentofu.org/scaleway/scaleway\"]",
	   "instances": [{"attributes": {"id": "fr-par-1/srv/nic", "server_id": "fr-par-1/srv", "private_network_id": "fr-par/pn"}}]},
	  {"mode": "managed", "type": "terraform_data", "name": "marker",
	   "provider": "provider[\"terraform.io/builtin/terraform\"]",
	   "instances": [{"attributes": {"id": "x"}}]},
	  {"mode": "data", "type": "scaleway_instance_image", "name": "debian",
	   "provider": "provider[\"registry.opentofu.org/scaleway/scaleway\"]",
	   "instances": [{"attributes": {"id": "fr-par-1/image"}}]}
	]}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, LiveStateFilename), []byte(state), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), []byte("# lock\n"), 0o600))
	return dir
}

// The configuration is what makes a broken stack undestroyable, so the
// fallback must bring none of it: only the providers the state names,
// the lock that pins them, and the workdir's installed plugins.
func TestRunWithoutConfigDestroysFromTheStateAlone(t *testing.T) {
	t.Parallel()
	workDir := withoutConfigWorkDir(t, withoutConfigProject)

	var got Command
	var mainTF, lock []byte
	runner := CommandRunnerFunc(func(_ context.Context, cmd Command) (CommandResult, error) {
		got = cmd
		mainTF, _ = os.ReadFile(filepath.Join(cmd.Dir, "main.tf"))
		lock, _ = os.ReadFile(filepath.Join(cmd.Dir, ".terraform.lock.hcl"))
		return CommandResult{Stdout: []byte("Destroy complete!")}, nil
	})

	out, err := NewSandboxDestroyHarness(runner).RunWithoutConfig(context.Background(), workDir, withoutConfigProject, map[string]string{"SCW_SECRET_KEY": "secret"})

	require.NoError(t, err)
	assert.Equal(t, WithoutConfigStage, out.Destroy.Stage)
	assert.Equal(t, []string{"destroy", "-auto-approve", "-state=" + filepath.Join(workDir, LiveStateFilename)}, got.Args)
	assert.Equal(t, filepath.Join(workDir, ".terraform"), got.Env["TF_DATA_DIR"], "reuse the providers init installed")
	assert.Equal(t, "secret", got.Env["SCW_SECRET_KEY"])
	assert.Equal(t, SandboxStripEnv, got.StripEnv, "the same sealed environment as every Layer 3 command")
	assert.NotEqual(t, workDir, got.Dir, "the workdir's own configuration is the thing that cannot evaluate")
	assert.Equal(t, `terraform {
  required_providers {
    scaleway = { source = "registry.opentofu.org/scaleway/scaleway" }
  }
}

provider "scaleway" {
  alias = "ams"
}

provider "scaleway" {}
`, string(mainTF))
	assert.Equal(t, "# lock\n", string(lock))
	assert.NoDirExists(t, got.Dir, "the temporary configuration is removed")
}

// Nothing the configuration says applies to this destroy, so it refuses
// anything it cannot place inside the run's own project.
func TestRunWithoutConfigRefusesOutsideTheRunProject(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		resourceProject, runProject, extra string
	}{
		"resource in another project": {resourceProject: "another-project", runProject: withoutConfigProject},
		"no run project":              {resourceProject: withoutConfigProject, runProject: ""},
		// The stray check skips projects -- their id is in `id`, not
		// `project_id` -- so a stale pre-ADR-0025 state could otherwise
		// aim this at another project.
		"a project": {resourceProject: withoutConfigProject, runProject: withoutConfigProject, extra: `
		  {"mode": "managed", "type": "scaleway_account_project", "name": "old",
		   "provider": "provider[\"registry.opentofu.org/scaleway/scaleway\"]",
		   "instances": [{"attributes": {"id": "another-project"}}]}`},
		// A child is placed only through parents in the same state.
		"a child whose parent is elsewhere": {resourceProject: withoutConfigProject, runProject: withoutConfigProject, extra: `
		  {"mode": "managed", "type": "scaleway_instance_private_nic", "name": "other",
		   "provider": "provider[\"registry.opentofu.org/scaleway/scaleway\"]",
		   "instances": [{"attributes": {"id": "fr-par-1/theirs/nic", "server_id": "fr-par-1/theirs", "private_network_id": "fr-par/pn"}}]}`},
		"a child whose parent was looked up": {resourceProject: withoutConfigProject, runProject: withoutConfigProject, extra: `
		  {"mode": "data", "type": "scaleway_vpc_private_network", "name": "theirs",
		   "provider": "provider[\"registry.opentofu.org/scaleway/scaleway\"]",
		   "instances": [{"attributes": {"id": "fr-par/their-pn"}}]},
		  {"mode": "managed", "type": "scaleway_instance_private_nic", "name": "other",
		   "provider": "provider[\"registry.opentofu.org/scaleway/scaleway\"]",
		   "instances": [{"attributes": {"id": "fr-par-1/srv/nic2", "server_id": "fr-par-1/srv", "private_network_id": "fr-par/their-pn"}}]}`},
		// No project_id and no parent in the stack: organization-wide.
		"an organization-scoped resource": {resourceProject: withoutConfigProject, runProject: withoutConfigProject, extra: `
		  {"mode": "managed", "type": "scaleway_iam_application", "name": "ci",
		   "provider": "provider[\"registry.opentofu.org/scaleway/scaleway\"]",
		   "instances": [{"attributes": {"id": "app", "organization_id": "org"}}]}`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runner := &fakeRunner{}
			var extra []string
			if tc.extra != "" {
				extra = append(extra, tc.extra)
			}

			_, err := NewSandboxDestroyHarness(runner).RunWithoutConfig(context.Background(), withoutConfigWorkDir(t, tc.resourceProject, extra...), tc.runProject, nil)

			require.ErrorIs(t, err, ErrProtectedProject)
			assert.Empty(t, runner.calls, "refused before tofu runs")
		})
	}
}
