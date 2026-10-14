package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
// holds a comma. prepare runs the fakeaws fakeawsAtPin built, passed as
// --fakeaws-bin, so nothing is installed under the scratch HOME, and
// leaves it running; each case stops it.
//
// The provider override names the endpoints keys the test reads itself
// from the locked provider's schema, at the reported fakeaws URL, and
// fakeaws answers the record's token once Execute has returned. The shell
// holds both proxies at a recorder, which no child of prepare may reach,
// and TF_LOG_PATH, which a tofu that inherits the shell writes: the schema
// call makes no request, and its env sets both proxies, so only the log
// shows it inheriting the shell.
func TestRehearsePrepare(t *testing.T) {
	e2e.SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	// Before HOME moves, so the build uses this machine's Go caches.
	bin := fakeawsAtPin(t)

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
			t.Cleanup(func() { stopRecordedFakeaws(t, repo) })
			// A HOME per case, so the check that prepare wrote nothing
			// there is this case's.
			home := realTempDir(t)
			t.Setenv("HOME", home)
			t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))

			// Both spellings, as git's libcurl reads only the lowercase
			// http_proxy, and no NO_PROXY that could let a child past it.
			proxy := startRecorder(t)
			for _, name := range []string{"HTTPS_PROXY", "HTTP_PROXY", "https_proxy", "http_proxy"} {
				t.Setenv(name, proxy.url)
			}
			t.Setenv("NO_PROXY", "")
			t.Setenv("no_proxy", "")
			tofuLog := filepath.Join(t.TempDir(), "tofu.log")
			t.Setenv("TF_LOG", "trace")
			t.Setenv("TF_LOG_PATH", tofuLog)

			root := cli.NewRootCmd()
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(stderr)
			root.SetArgs([]string{"rehearse", "prepare", "envs/prod", "--account", "123456789012", "--region", assumptionRegion, "--fakeaws-bin", bin, "--output", "json"})
			err := root.Execute()
			assert.Zero(t, proxy.count(), "a child of prepare went through the shell's proxy: %v", proxy.requests())
			assert.NoFileExists(t, tofuLog, "a tofu prepare ran inherited the shell's TF_LOG_PATH")
			require.NoError(t, err, stdout.String())

			var result struct {
				ID         string   `json:"id"`
				WorkDir    string   `json:"work_dir"`
				FakeawsURL string   `json:"fakeaws_url"`
				Version    string   `json:"aws_provider_version"`
				Warnings   []string `json:"warnings"`
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
			// Before this test runs tofu in the rehearsal itself.
			assertTightModes(t, rehearsal)
			for _, dir := range []string{"runs", "live"} {
				assert.NoDirExists(t, filepath.Join(repo, ".infrafactory", dir))
			}
			homeEntries, err := os.ReadDir(home)
			require.NoError(t, err)
			assert.Empty(t, homeEntries, "prepare wrote under HOME, so it did not run --fakeaws-bin")

			record := readPrepareRecord(t, rehearsal)
			require.Len(t, record.Roots, 1)
			require.NotEmpty(t, record.Token)
			assert.NotContains(t, stdout.String(), record.Token, "prepare printed the admin token")
			assert.NotContains(t, stderr.String(), record.Token, "prepare printed the admin token on stderr")
			assert.Equal(t, fmt.Sprintf("http://127.0.0.1:%d", record.Port), result.FakeawsURL)
			assert.Equal(t, result.WorkDir, filepath.Join(rehearsal, record.Roots[0].Root))
			keys := schemaEndpointKeys(t, result.WorkDir, []string{
				"HOME=" + t.TempDir(),
				"TF_DATA_DIR=" + data,
				"HTTPS_PROXY=http://" + closedAddr,
				"HTTP_PROXY=http://" + closedAddr,
			}, tc.name)
			assert.Equal(t, keys, record.Roots[0].EndpointKeys)
			awsOverride, err := os.ReadFile(filepath.Join(result.WorkDir, cli.AWSOverride))
			require.NoError(t, err)
			assert.Equal(t, string(cli.RehearsalProviderOverride(result.FakeawsURL, record.Region, keys, record.Roots[0].AWSAliases)), string(awsOverride))
			assert.Equal(t, http.StatusOK, mockRequestsStatus(t, record.Port, record.Token), "fakeaws did not outlive prepare")
		})
	}
}

// prepareRecord is what TestRehearsePrepare reads of rehearsal.json.
type prepareRecord struct {
	Region string `json:"region"`
	PID    int    `json:"pid"`
	Port   int    `json:"port"`
	Token  string `json:"token"`
	Roots  []struct {
		Root         string   `json:"root"`
		EndpointKeys []string `json:"endpoint_keys"`
		AWSAliases   []string `json:"aws_aliases"`
	} `json:"roots"`
}

func readPrepareRecord(t *testing.T, rehearsal string) prepareRecord {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(rehearsal, "rehearsal.json"))
	require.NoError(t, err)
	var record prepareRecord
	require.NoError(t, json.Unmarshal(raw, &record), string(raw))
	return record
}

// mockRequestsStatus is the status of GET /mock/requests on the loopback
// port, with token as the bearer, through no proxy.
func mockRequestsStatus(t *testing.T, port int, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/mock/requests", port), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{}}).Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	return resp.StatusCode
}

// fakeawsRepo is where fakeawsAtPin clones fakeaws from.
const fakeawsRepo = "https://" + cli.RehearsalFakeawsModule + ".git"

// fakeawsBuildTimeout bounds each git and go command fakeawsAtPin runs.
const fakeawsBuildTimeout = 5 * time.Minute

// fakeawsAtPin builds fakeaws's cmd/fakeaws from FAKEAWS_DIR into a
// scratch dir and returns the binary. Unset, FAKEAWS_DIR is a fresh clone
// at the pin under t.TempDir(), never the shared ../fakeaws, which every
// worktree sees at whatever commit it was left at. Either way the dir's
// HEAD must be the pin. -buildvcs=true stamps the commit prepare's build
// info check reads, or fails the build.
func fakeawsAtPin(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("FAKEAWS_DIR")
	if dir == "" {
		dir = filepath.Join(t.TempDir(), "fakeaws")
		for _, args := range [][]string{
			{"init", "-q", dir},
			{"-C", dir, "fetch", "-q", "--depth", "1", fakeawsRepo, cli.RehearsalFakeawsPin},
			{"-C", dir, "checkout", "-q", "FETCH_HEAD"},
		} {
			fakeawsBuildStep(t, "", "git", args...)
		}
	}
	require.NoError(t, fakeawsHeadAtPin(dir, cli.RehearsalFakeawsPin))
	bin := filepath.Join(t.TempDir(), "fakeaws")
	fakeawsBuildStep(t, dir, "go", "build", "-buildvcs=true", "-o", bin, "./cmd/fakeaws")
	return bin
}

// fakeawsBuildStep runs name in dir under fakeawsBuildTimeout, and fails
// the test on any error. It inherits this process's environment but no
// GIT_* (a GIT_DIR would point git at another repository), no workspace
// and no GOFLAGS of the shell's or the go env file's, so go builds the
// tree in dir as it is. GOFLAGS is set, not emptied: an empty one would
// still read the go env file's.
func fakeawsBuildStep(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), fakeawsBuildTimeout)
	defer cancel()
	env := map[string]string{"GOWORK": "off", "GOFLAGS": "-mod=readonly"}
	result, err := cli.ExecRunner.Run(ctx, harness.Command{Name: name, Args: args, Dir: dir, Env: env, StripEnv: fakeawsGitEnv})
	require.NoError(t, err, "%s %s\n%s", name, strings.Join(args, " "), result.Stderr)
}

// fakeawsGitEnv is what fakeawsAtPin's commands drop from the inherited
// environment.
var fakeawsGitEnv = []string{"GIT_*"}

// fakeawsHeadAtPin admits dir only when its git HEAD is pin; anything
// else, a failed rev-parse included, is refused, naming both commits.
func fakeawsHeadAtPin(dir, pin string) error {
	ctx, cancel := context.WithTimeout(context.Background(), fakeawsBuildTimeout)
	defer cancel()
	result, err := cli.ExecRunner.Run(ctx, harness.Command{Name: "git", Args: []string{"-C", dir, "rev-parse", "HEAD"}, StripEnv: fakeawsGitEnv})
	if err != nil {
		return fmt.Errorf("FAKEAWS_DIR %s has no HEAD to hold against the pin %s: %w\n%s", dir, pin, err, result.Stderr)
	}
	if head := strings.TrimSpace(string(result.Stdout)); head != pin {
		return fmt.Errorf("FAKEAWS_DIR %s is at %s, not the pin %s", dir, head, pin)
	}
	return nil
}

// A FAKEAWS_DIR whose HEAD is not the pin is refused, naming both commits;
// its own HEAD is admitted.
func TestFakeawsHeadAtPinRefusesAnotherCommit(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := runGit(t, map[string]string{"GIT_CONFIG_NOSYSTEM": "1"}, append([]string{"-C", dir}, args...)...)
		require.NoError(t, err, out)
		return strings.TrimSpace(out)
	}
	git("init", "-q")
	git("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "not the pin")
	head := git("rev-parse", "HEAD")

	err := fakeawsHeadAtPin(dir, cli.RehearsalFakeawsPin)
	require.Error(t, err)
	assert.ErrorContains(t, err, head)
	assert.ErrorContains(t, err, cli.RehearsalFakeawsPin)
	assert.NoError(t, fakeawsHeadAtPin(dir, head))
}

// stopRecordedFakeaws stops each fakeaws recorded under repo's rehearsals
// and waits until its process group is gone, so t.TempDir's removal never
// races a fakeaws still holding its db files.
func stopRecordedFakeaws(t *testing.T, repo string) {
	records, _ := filepath.Glob(filepath.Join(repo, ".infrafactory", "rehearsals", "*", "rehearsal.json"))
	for _, path := range records {
		raw, err := os.ReadFile(path)
		if !assert.NoError(t, err) {
			continue
		}
		var record prepareRecord
		if assert.NoError(t, json.Unmarshal(raw, &record), path) && assert.Positive(t, record.PID, path) {
			assert.NoError(t, cli.StopRehearsalFakeaws(record.PID, record.Port), path)
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
