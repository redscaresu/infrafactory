package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/cli"
	"github.com/redscaresu/infrafactory/internal/e2e"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// TestRehearsePrepare runs the whole `rehearse prepare` with real tofu, at
// the default dead proxy, on a repo whose root has a lock and either calls
// one local module or has no module blocks, so gate 2 runs on what tofu
// wrote. After both inits the aws provider is under <id>/data at the
// lock's version, modules.json names the local module, the user's lock is
// unchanged, the backend override is in the copy's root, tofu initialised
// the local backend with its state under <id>/state, and every mode in
// the rehearsal is tightened. A lock whose aws entry is
// registry.terraform.io's prepares too, with the warning. The repo's path
// holds a comma. prepare installs fakeaws at the pin into a scratch user
// cache, shared by the cases, and leaves it running; each case stops it.
func TestRehearsePrepare(t *testing.T) {
	e2e.SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	cache := realTempDir(t)
	t.Setenv("HOME", cache)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(cache, "cache"))

	const module = "\nmodule \"m\" {\n  source = \"../../modules/m\"\n}\n"
	for _, tc := range []struct {
		name, addr, module string
	}{
		{"a local module", "registry.opentofu.org/hashicorp/aws", module},
		{"a local module, a registry.terraform.io lock", "registry.terraform.io/hashicorp/aws", module},
		{"no module blocks", "registry.opentofu.org/hashicorp/aws", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A comma in the path: tofu 1.13 splits -plugin-dir's value at
			// one.
			repo := filepath.Join(realTempDir(t), "a,repo")
			lock := fmt.Sprintf("provider %q {\n  version     = %q\n  constraints = %q\n}\n", tc.addr, harness.AWSProviderVersion, harness.AWSProviderVersion)
			for name, content := range map[string]string{
				".git/HEAD":                     "ref: refs/heads/main\n",
				"envs/prod/.terraform.lock.hcl": lock,
				"envs/prod/main.tf": fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = %q
    }
  }
}

provider "aws" {
  region = %q
}
`, harness.AWSProviderVersion, assumptionRegion) + tc.module,
				"modules/m/main.tf": manifestOutput,
			} {
				e2e.WriteFile(t, filepath.Join(repo, name), []byte(content))
			}
			t.Chdir(repo)
			t.Cleanup(func() { stopRecordedFakeaws(repo) })

			root := cli.NewRootCmd()
			stdout := &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(&bytes.Buffer{})
			root.SetArgs([]string{"rehearse", "prepare", "envs/prod", "--account", "123456789012", "--region", assumptionRegion, "--output", "json"})
			require.NoError(t, root.Execute(), stdout.String())

			var result struct {
				ID       string   `json:"id"`
				WorkDir  string   `json:"work_dir"`
				Version  string   `json:"aws_provider_version"`
				Warnings []string `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &result), stdout.String())
			rehearsal := filepath.Join(repo, ".infrafactory", "rehearsals", result.ID)
			data := filepath.Join(rehearsal, "data")
			assert.Equal(t, harness.AWSProviderVersion, result.Version)
			assert.DirExists(t, filepath.Join(data, "providers", "registry.opentofu.org", "hashicorp", "aws", harness.AWSProviderVersion))
			if tc.module != "" {
				assert.Contains(t, prepareManifest(t, data), manifestEntry{"m", "../../modules/m", "../../modules/m"})
			}
			override, err := os.ReadFile(filepath.Join(result.WorkDir, cli.OurOverride))
			require.NoError(t, err)
			assert.Equal(t, cli.LocalBackendOverride, string(override))
			assertLocalBackendAt(t, data, filepath.Join(rehearsal, "state", "terraform.tfstate"), filepath.Join(rehearsal, "work"))
			userLock, err := os.ReadFile(filepath.Join(repo, "envs", "prod", ".terraform.lock.hcl"))
			require.NoError(t, err)
			assert.Equal(t, lock, string(userLock), "prepare changed the user's lock")
			if tc.addr == "registry.terraform.io/hashicorp/aws" {
				require.Len(t, result.Warnings, 1)
				assert.Contains(t, result.Warnings[0], "registry.terraform.io's")
			}
			assertTightModes(t, rehearsal)
		})
	}
}

// stopRecordedFakeaws kills the process group of each fakeaws recorded
// under repo's rehearsals.
func stopRecordedFakeaws(repo string) {
	records, _ := filepath.Glob(filepath.Join(repo, ".infrafactory", "rehearsals", "*", "rehearsal.json"))
	for _, path := range records {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var record struct {
			PID int `json:"pid"`
		}
		if json.Unmarshal(raw, &record) == nil && record.PID > 0 {
			_ = syscall.Kill(-record.PID, syscall.SIGKILL)
		}
	}
}

// assertLocalBackendAt reads the backend config the second init wrote at
// <data>/terraform.tfstate: tofu initialised the local backend with its
// state at state, which lies outside work.
func assertLocalBackendAt(t *testing.T, data, state, work string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(data, "terraform.tfstate"))
	require.NoError(t, err)
	var config struct {
		Backend struct {
			Type   string
			Config struct{ Path string }
		}
	}
	require.NoError(t, json.Unmarshal(raw, &config), string(raw))
	assert.Equal(t, "local", config.Backend.Type, string(raw))
	assert.Equal(t, state, config.Backend.Config.Path, string(raw))
	rel, err := filepath.Rel(work, config.Backend.Config.Path)
	require.NoError(t, err)
	assert.False(t, filepath.IsLocal(rel), "the state %s lies inside %s", config.Backend.Config.Path, work)
}

// prepareManifest is the module manifest the first init wrote under data.
func prepareManifest(t *testing.T, data string) []manifestEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(data, "modules", "modules.json"))
	require.NoError(t, err)
	var manifest struct{ Modules []manifestEntry }
	require.NoError(t, json.Unmarshal(raw, &manifest), string(raw))
	return manifest.Modules
}

// assertTightModes walks dir: every dir is 0700, every provider binary
// 0700, every other file 0600, and nothing else is there.
func assertTightModes(t *testing.T, dir string) {
	t.Helper()
	binaries := 0
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		info, err := d.Info()
		require.NoError(t, err)
		switch {
		case d.IsDir():
			assert.Equal(t, fs.ModeDir|0o700, info.Mode(), path)
		case !info.Mode().IsRegular():
			t.Errorf("%s is %v, not a dir or a regular file", path, info.Mode())
		case strings.HasPrefix(d.Name(), "terraform-provider-"):
			binaries++
			assert.Equal(t, fs.FileMode(0o700), info.Mode(), path)
		default:
			assert.Equal(t, fs.FileMode(0o600), info.Mode(), path)
		}
		return nil
	}))
	assert.Positive(t, binaries, "no provider binary under %s", dir)
}
