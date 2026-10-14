package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/cli"
	"github.com/redscaresu/infrafactory/internal/e2e"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// wallsFailDeadline is how long a tofu step a wall stops may take to die.
const wallsFailDeadline = 60 * time.Second

// TestRehearsalWallsFailClosed runs a real prepare on a root with a user
// backend "s3", an aws_vpc (EC2, which fakeaws models) and an aws_lb
// (ELBv2, which it does not), then bare tofu in the work dir. The shell
// holds AWS's example keys, AWS_PROFILE naming a profile with real-looking
// keys in a planted HOME/.aws, and both proxies at a recorder.
//
// With every wall, the vpc lands on fakeaws and the lb's ELBv2 call is
// dialled straight at the dead proxy and refused. Remove the provider
// override, the environment wall or the backend override, and tofu still
// dies, within wallsFailDeadline: the ELBv2 call at the dead proxy through
// the walls env's AWS_ENDPOINT_URL catch-all, then through the override's
// elbv2 key, and the backend's S3 call there too. No case dials a proxy or
// reaches the recorder.
func TestRehearsalWallsFailClosed(t *testing.T) {
	e2e.SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	// Before HOME moves, so the build uses this machine's Go caches.
	bin := fakeawsAtPin(t)

	repo := wallsRepo(t)
	proxy, _ := prodShell(t)

	root := cli.NewRootCmd()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"rehearse", "prepare", wallsRootDir, "--account", wallsAccount, "--region", assumptionRegion, "--fakeaws-bin", bin, "--output", "json"})
	require.NoError(t, root.Execute(), "%s\n%s", stdout, stderr)
	require.Zero(t, proxy.count(), "prepare went through the shell's proxy: %v", proxy.requests())
	var result struct {
		ID         string `json:"id"`
		WorkDir    string `json:"work_dir"`
		FakeawsURL string `json:"fakeaws_url"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result), stdout.String())
	rehearsal := filepath.Join(repo, ".infrafactory", "rehearsals", result.ID)
	record := readPrepareRecord(t, rehearsal)
	walls := cli.RehearsalWallsEnv(rehearsal, result.FakeawsURL, record.Region)
	elbv2Dead := deadCall(t, "Elastic Load Balancing v2", "Post")

	w := wallsDir{t: t, dir: result.WorkDir, proxy: proxy}
	t.Run("every wall", func(t *testing.T) {
		w := w.in(t)
		out, err := w.tofu(walls, "apply", "-auto-approve", "-input=false", "-no-color", "-target=aws_vpc.main")
		require.NoError(t, err, out)
		assert.Contains(t, fakeawsActions(t, record.Port, record.Token), "CreateVpc")
		w.dies(walls, elbv2Dead, wallsApplyLB...)
	})
	t.Run("no provider override", func(t *testing.T) {
		w := w.in(t)
		w.remove(cli.AWSOverride)
		w.dies(walls, elbv2Dead, wallsApplyLB...)
	})
	t.Run("no environment wall", func(t *testing.T) {
		w := w.in(t)
		// The providers stay installed; every AWS_* and proxy variable
		// is the shell's.
		env := map[string]string{"HOME": walls["HOME"], "TF_DATA_DIR": walls["TF_DATA_DIR"]}
		for _, entry := range os.Environ() {
			key, value, _ := strings.Cut(entry, "=")
			switch strings.ToUpper(key) {
			case "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY":
				env[key] = value
			}
			if strings.HasPrefix(key, "AWS_") {
				env[key] = value
			}
		}
		require.Equal(t, fakeAccessKeyID, env["AWS_ACCESS_KEY_ID"])
		require.Equal(t, "prod", env["AWS_PROFILE"])
		require.Equal(t, proxy.url, env["HTTPS_PROXY"])
		w.dies(env, elbv2Dead, wallsApplyLB...)
	})
	t.Run("no backend override", func(t *testing.T) {
		w := w.in(t)
		w.remove(cli.OurOverride)
		w.dies(walls, regexp.MustCompile(regexp.QuoteMeta(`Backend type changed from "local" to "s3"`)), "apply", "-auto-approve", "-input=false", "-no-color")
		w.dies(walls, deadCall(t, "S3", "Get"), "init", "-input=false", "-no-color", "-reconfigure", "-plugin-dir="+filepath.Join(walls["TF_DATA_DIR"], "providers"))
	})
}

// wallsAccount is the account the walls tests prepare for, and the
// teardown test's stand-in prod is.
const wallsAccount = "123456789012"

// wallsRootDir is wallsRoot's dir in the repo wallsRepo makes.
const wallsRootDir = "envs/prod"

// wallsRepo makes a repo holding wallsRoot at wallsRootDir, with a lock
// at the harness's aws provider version, makes it the working dir, and
// stops any fakeaws a rehearsal of it leaves running when the test ends.
// It returns the repo.
func wallsRepo(t *testing.T) string {
	t.Helper()
	repo := realTempDir(t)
	lock := fmt.Sprintf("provider %q {\n  version     = %q\n  constraints = %q\n}\n", "registry.opentofu.org/hashicorp/aws", harness.AWSProviderVersion, harness.AWSProviderVersion)
	for name, content := range map[string]string{
		".git/HEAD": "ref: refs/heads/main\n",
		filepath.Join(wallsRootDir, ".terraform.lock.hcl"): lock,
		filepath.Join(wallsRootDir, "main.tf"):             wallsRoot,
	} {
		e2e.WriteFile(t, filepath.Join(repo, name), []byte(content))
	}
	t.Chdir(repo)
	t.Cleanup(func() { stopRecordedFakeaws(t, repo) })
	return repo
}

// prodShell sets the shell of a user who can reach prod: AWS's example
// keys, AWS_PROFILE naming a profile with real-looking keys in a planted
// HOME/.aws, and both proxies, in both spellings, at the recorder it
// returns, with no NO_PROXY to let a child past it. It returns the
// variables it set too.
func prodShell(t *testing.T) (*recorder, map[string]string) {
	t.Helper()
	home := realTempDir(t)
	e2e.WriteFile(t, filepath.Join(home, ".aws", "credentials"), []byte(
		"[prod]\naws_access_key_id = "+fakeAccessKeyID+"\naws_secret_access_key = "+fakeSecretAccessKey+"\n"))
	e2e.WriteFile(t, filepath.Join(home, ".aws", "config"), []byte("[profile prod]\nregion = "+assumptionRegion+"\n"))
	proxy := startRecorder(t)
	shell := map[string]string{
		"HOME":                  home,
		"XDG_CACHE_HOME":        filepath.Join(home, "cache"),
		"AWS_ACCESS_KEY_ID":     fakeAccessKeyID,
		"AWS_SECRET_ACCESS_KEY": fakeSecretAccessKey,
		"AWS_PROFILE":           "prod",
		// Named too, so the profile still resolves where HOME is the
		// rehearsal's.
		"AWS_CONFIG_FILE":             filepath.Join(home, ".aws", "config"),
		"AWS_SHARED_CREDENTIALS_FILE": filepath.Join(home, ".aws", "credentials"),
		"NO_PROXY":                    "",
		"no_proxy":                    "",
	}
	for _, name := range []string{"HTTPS_PROXY", "HTTP_PROXY", "https_proxy", "http_proxy"} {
		shell[name] = proxy.url
	}
	for name, value := range shell {
		t.Setenv(name, value)
	}
	return proxy, shell
}

// wallsRoot is the user's root: a backend "s3", a provider with
// max_retries = 1, so a refused request fails fast, an aws_vpc and an
// aws_lb that does not depend on it, in a subnet named literally.
var wallsRoot = fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = %q
    }
  }

  backend "s3" {
    bucket = "prod-state"
    key    = "prod.tfstate"
    region = %q
  }
}

`, harness.AWSProviderVersion, assumptionRegion) + providerBlock(fmt.Sprintf("  region = %q\n", assumptionRegion)) + `resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_lb" "main" {
  name               = "rehearsal"
  internal           = true
  load_balancer_type = "network"
  subnets            = ["subnet-00000000000000000"]
}
`

// wallsApplyLB applies the aws_lb alone.
var wallsApplyLB = []string{"apply", "-auto-approve", "-input=false", "-no-color", "-target=aws_lb.main"}

// wallsDir is the prepared work dir one case runs bare tofu in, and the
// shell's proxy recorder with its count when the case began.
type wallsDir struct {
	t     *testing.T
	dir   string
	proxy *recorder
	seen  int
}

// in is w for the case t, which fails if the recorder counts a request
// before it ends.
func (w wallsDir) in(t *testing.T) wallsDir {
	w.t, w.seen = t, w.proxy.count()
	t.Cleanup(func() {
		assert.Equal(t, w.seen, w.proxy.count(), "a request reached the shell's proxy: %v", w.proxy.requests())
	})
	return w
}

// tofu runs tofu in the work dir with env as the whole of its
// environment besides PATH, and returns stdout and stderr together.
func (w wallsDir) tofu(env map[string]string, args ...string) (string, error) {
	w.t.Helper()
	vars := make([]string, 0, len(env))
	for key, value := range env {
		vars = append(vars, key+"="+value)
	}
	stdout, stderr, err := cli.RunTofu(w.dir, vars, args...)
	return stdout + stderr, err
}

// dies runs tofu, which must fail within wallsFailDeadline with an error
// want matches, and with no proxied dial: every proxy a case sets is the
// dead proxy or the recorder.
func (w wallsDir) dies(env map[string]string, want *regexp.Regexp, args ...string) {
	w.t.Helper()
	start := time.Now()
	out, err := w.tofu(env, args...)
	elapsed := time.Since(start)
	var errs []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "Error: ") {
			errs = append(errs, line)
		}
	}
	w.t.Logf("tofu %s died in %s:\n%s", strings.Join(args, " "), elapsed.Round(time.Millisecond), strings.Join(errs, "\n"))
	require.Error(w.t, err, "tofu %s succeeded\n%s", strings.Join(args, " "), out)
	assert.Less(w.t, elapsed, wallsFailDeadline, "tofu %s took %s to die", strings.Join(args, " "), elapsed)
	assert.Regexp(w.t, want, out, "tofu %s", strings.Join(args, " "))
	assert.NotContains(w.t, out, "proxyconnect", "tofu %s dialled a proxy", strings.Join(args, " "))
}

// deadCall matches an error line on which a call to svc, as the AWS SDK
// names the service, was sent to the dead proxy itself and refused there.
// op is the request's method as net/http's url.Error spells it (Post).
// Through a proxy, the line names the service's own URL and proxyconnect
// instead; a call to another service names that service.
func deadCall(t *testing.T, svc, op string) *regexp.Regexp {
	t.Helper()
	dead, err := url.Parse(cli.RehearsalDeadProxy)
	require.NoError(t, err)
	return regexp.MustCompile(regexp.QuoteMeta("operation error "+svc+": ") + `\w+, .*` +
		regexp.QuoteMeta(op+` "`+cli.RehearsalDeadProxy+"/") + `[^"]*": ` + regexp.QuoteMeta("dial tcp "+dead.Host+":"))
}

// remove takes name out of the work dir for the case, and puts it back
// after.
func (w wallsDir) remove(name string) {
	w.t.Helper()
	path := filepath.Join(w.dir, name)
	content, err := os.ReadFile(path)
	require.NoError(w.t, err)
	require.NoError(w.t, os.Remove(path))
	w.t.Cleanup(func() { assert.NoError(w.t, os.WriteFile(path, content, 0o600)) })
}

// fakeawsActions is the Action of each request in fakeaws's request log,
// read with the admin token through no proxy.
func fakeawsActions(t *testing.T, port int, token string) []string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/mock/requests", port), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{}}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var log struct {
		Requests []struct {
			Action string `json:"action"`
		} `json:"requests"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&log))
	actions := make([]string, 0, len(log.Requests))
	for _, r := range log.Requests {
		actions = append(actions, r.Action)
	}
	return actions
}
