package cli_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/cli"
	"github.com/redscaresu/infrafactory/internal/e2e"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// The tofu and provider behaviour the rehearsal design rests on (ADR-0028:
// a claim is a test). Each test runs real tofu with the provider the
// harness pins, so a tofu or provider upgrade that breaks a claim fails.

const (
	assumptionRegion = "us-east-1"
	// closedAddr is the discard port, which nothing serves by default or
	// on CI; newAssumptionDir checks that it refuses a connection.
	closedAddr     = "127.0.0.1:9"
	closedEndpoint = "http://" + closedAddr
	closedDial     = "dial tcp " + closedAddr + ":"
	// AWS's documented example key pair: real-looking, never valid.
	fakeAccessKeyID     = "AKIAIOSFODNN7EXAMPLE"
	fakeSecretAccessKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
)

const aliasConflict = "Only one of the following attributes should be set"

// TestAssumption_EndpointsOverrideKeepsApplyOffProd: a provider override
// naming every key of the `endpoints` block, STS included, makes `tofu
// apply` with real-looking keys in the environment die at the closed
// loopback port, and nothing reaches a non-loopback host.
//
// Relied on by: rehearsal-walls (HLD § Goals, goal 7; § Risks and safety,
// "A rehearsal that reaches prod"). ADR-0028.
func TestAssumption_EndpointsOverrideKeepsApplyOffProd(t *testing.T) {
	w := newAssumptionDir(t)
	keys := w.endpointKeys(t)
	override := endpointsOverride(keys, closedEndpoint)
	assert.Len(t, keys, countEndpoints(t, override),
		"the override names every key of the schema's endpoints block (%s)", w.version)
	t.Logf("%s: the override names %d endpoints keys", w.version, len(keys))

	t.Run("sts", func(t *testing.T) {
		proxy := startRecorder(t)
		w.write(t, "main.tf", providerBlock("")+`data "aws_caller_identity" "me" {}`+"\n")
		w.write(t, cli.OurOverride, override)

		errs, aliasWarnings := w.applyJSON(t, proxy)
		assert.Positive(t, aliasWarnings, "%s: no %q warning: reworded, or now an error?", w.version, aliasConflict)
		assert.Contains(t, errs, "GetCallerIdentity", w.version)
		assert.Contains(t, errs, closedDial, "%s: %s", w.version, errs)
		assert.Zero(t, proxy.count(), "%s: requests left loopback: %v", w.version, proxy.requests())
	})

	t.Run("ec2", func(t *testing.T) {
		proxy := startRecorder(t)
		w.write(t, "main.tf", providerBlock("  skip_credentials_validation = true\n  skip_requesting_account_id  = true\n")+
			`data "aws_vpcs" "all" {}`+"\n")
		w.write(t, cli.OurOverride, override)

		errs, aliasWarnings := w.applyJSON(t, proxy)
		assert.Positive(t, aliasWarnings, "%s: no %q warning: reworded, or now an error?", w.version, aliasConflict)
		assert.Contains(t, errs, "DescribeVpcs", w.version)
		assert.Contains(t, errs, closedDial, "%s: %s", w.version, errs)
		assert.Zero(t, proxy.count(), "%s: requests left loopback: %v", w.version, proxy.requests())
	})

	t.Run("control without sts", func(t *testing.T) {
		proxy := startRecorder(t)
		w.write(t, "main.tf", providerBlock("")+`data "aws_caller_identity" "me" {}`+"\n")
		w.write(t, cli.OurOverride, endpointsOverride(without(keys, "sts"), closedEndpoint))

		errs, _ := w.applyJSON(t, proxy)
		assert.Positive(t, proxy.count(), "%s: an STS key the override leaves out goes to AWS: %s", w.version, errs)
		t.Logf("%s: the proxy saw %v", w.version, proxy.requests())
	})
}

// TestAssumption_UserOverrideMergesAfterOurs: tofu merges override files
// in name order, so a user's zz_override.tf merges after infrafactory's
// and its `endpoints` wins.
//
// Relied on by: rehearsal-prepare-gate, which refuses user override files
// (HLD § Prepare: two work directories, gated before any tofu). ADR-0028.
func TestAssumption_UserOverrideMergesAfterOurs(t *testing.T) {
	w := newAssumptionDir(t)
	proxy, userEndpoint := startRecorder(t), startRecorder(t)
	w.write(t, "main.tf", providerBlock("")+`data "aws_caller_identity" "me" {}`+"\n")
	w.write(t, cli.OurOverride, endpointsOverride([]string{"sts"}, closedEndpoint))
	w.write(t, "zz_override.tf", endpointsOverride([]string{"sts"}, userEndpoint.url))

	out, _ := w.tofu(t, w.env(t, proxy), "apply", "-auto-approve", "-input=false", "-no-color")
	assert.Positive(t, userEndpoint.count(), "%s: the user's STS endpoint got nothing\n%s", w.version, out)
	assert.NotContains(t, out, closedDial, w.version)
	assert.Zero(t, proxy.count(), "%s: requests left loopback: %v", w.version, proxy.requests())
}

// TestAssumption_RootOverrideMissesChildModuleProvider: a root override of
// `provider "aws"` does not reach a `provider "aws"` declared inside a
// child module, whose requests follow the environment instead.
//
// Relied on by: rehearsal-prepare-gate, which refuses a provider block
// inside a child module (HLD § Prepare: two work directories, gated
// before any tofu). ADR-0028.
func TestAssumption_RootOverrideMissesChildModuleProvider(t *testing.T) {
	w := newAssumptionDir(t)
	proxy, root, child := startRecorder(t), startRecorder(t), startRecorder(t)
	w.write(t, "main.tf", providerBlock("")+`module "child" { source = "./child" }`+"\n")
	w.write(t, cli.OurOverride, endpointsOverride([]string{"sts"}, root.url))
	w.write(t, "child/main.tf", providerBlock("")+`data "aws_caller_identity" "me" {}`+"\n")
	w.init(t)

	out, _ := w.tofu(t, w.env(t, proxy, "AWS_ENDPOINT_URL_STS="+child.url),
		"apply", "-auto-approve", "-input=false", "-no-color")
	assert.Positive(t, child.count(), "%s: the child's STS request went elsewhere\n%s", w.version, out)
	assert.Zero(t, root.count(), "%s: the root override reached the child: %v", w.version, root.requests())
	assert.Zero(t, proxy.count(), "%s: requests left loopback: %v", w.version, proxy.requests())
}

// TestAssumption_LoaderIgnoresUnknownSuffixes: tofu's configuration
// loader reads only its own configuration suffixes from a module
// directory, and no subdirectory but the test directory. A root with one
// valid output beside broken HCL in a.hcl, a.tf.bak, notes.txt,
// x.tfvars.example and sub/b.tf passes `tofu init -backend=false` and
// `tofu validate`, offline; broken HCL in tests/a.tftest.hcl fails both.
//
// Relied on by: rehearsal-prepare-gate, which admits any other file
// unparsed and refuses a tests entry (ADR-0042 § The extension and file
// policy). ADR-0028.
func TestAssumption_LoaderIgnoresUnknownSuffixes(t *testing.T) {
	e2e.SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")

	w, env := newLoaderRoot(t, "a.hcl", "a.tf.bak", "notes.txt", "x.tfvars.example", "sub/b.tf")
	out, err := w.tofu(t, env, "init", "-backend=false", "-input=false", "-no-color")
	require.NoError(t, err, "%s: init loaded a file it should ignore\n%s", w.version, out)
	out, err = w.tofu(t, env, "validate", "-no-color")
	assert.NoError(t, err, "%s: validate loaded a file it should ignore\n%s", w.version, out)

	t.Run("the test directory is read", func(t *testing.T) {
		w, env := newLoaderRoot(t, "tests/a.tftest.hcl")
		for _, args := range [][]string{{"init", "-backend=false", "-input=false", "-no-color"}, {"validate", "-no-color"}} {
			out, err := w.tofu(t, env, args...)
			assert.Error(t, err, "%s: tofu %s ignored the test directory\n%s", w.version, args[0], out)
			assert.Contains(t, out, "tests/a.tftest.hcl", "%s: tofu %s", w.version, args[0])
		}
	})
}

// newLoaderRoot is a root with one valid output plus broken HCL in each
// of broken, and the offline environment to run tofu in: both proxies at
// the closed port, so any download fails.
func newLoaderRoot(t *testing.T, broken ...string) (assumptionDir, []string) {
	t.Helper()
	w := assumptionDir{dir: t.TempDir()}
	w.write(t, "main.tf", "output \"x\" {\n  value = 1\n}\n")
	for _, name := range broken {
		w.write(t, name, "resource \"broken\" {\n")
	}
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "TF_DATA_DIR=" + t.TempDir(),
		"HTTPS_PROXY=" + closedEndpoint, "HTTP_PROXY=" + closedEndpoint}
	version, err := w.tofu(t, env, "version")
	require.NoError(t, err, version)
	w.version = strings.TrimSpace(version)
	return w, env
}

// TestAssumption_ModulesManifestShape: `tofu init -backend=false`, offline
// with TF_DATA_DIR outside the root, writes the module manifest at
// <TF_DATA_DIR>/modules/modules.json and nothing under the root. The root
// entry is Key "", Source "", Dir "." A local entry's Key is its module
// path joined by dots, its Source is as written, and its Dir is relative
// to the root and cleaned: m's ../shared is "shared". A root with no
// module blocks gets no manifest. A module fetched from an absolute path
// gets an absolute Dir under <TF_DATA_DIR>/modules, and a local module it
// calls an absolute Dir at the path it was fetched from.
//
// Relied on by: rehearsal-prepare-gate's gate 2, which reads the manifest
// (ADR-0042 Decision 2). ADR-0028.
func TestAssumption_ModulesManifestShape(t *testing.T) {
	e2e.SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")

	t.Run("local modules", func(t *testing.T) {
		tmp := realTempDir(t)
		entries, version := manifestInit(t, tmp, map[string]string{
			"root/main.tf":        "module \"m\" {\n  source = \"./m\"\n}\n",
			"root/m/main.tf":      "module \"s\" {\n  source = \"../shared\"\n}\n",
			"root/shared/main.tf": manifestOutput,
		})
		assert.ElementsMatch(t, []manifestEntry{{"", "", "."}, {"m", "./m", "m"}, {"m.s", "../shared", "shared"}}, entries, version)
		assert.NoDirExists(t, filepath.Join(tmp, "root", ".terraform"), version)
	})

	t.Run("no module blocks", func(t *testing.T) {
		entries, version := manifestInit(t, realTempDir(t), map[string]string{"root/main.tf": manifestOutput})
		assert.Nil(t, entries, "%s: a root with no module blocks got a manifest", version)
	})

	t.Run("a module fetched from an absolute path", func(t *testing.T) {
		tmp := realTempDir(t)
		ext := filepath.Join(tmp, "ext")
		entries, version := manifestInit(t, tmp, map[string]string{
			"root/main.tf":    fmt.Sprintf("module \"e\" {\n  source = %q\n}\n", ext),
			"ext/main.tf":     "module \"in\" {\n  source = \"./sub\"\n}\n",
			"ext/sub/main.tf": manifestOutput,
		})
		dirs := map[string]string{}
		for _, entry := range entries {
			dirs[entry.Key] = entry.Dir
		}
		assert.Equal(t, filepath.Join(tmp, "data", "modules", "e"), dirs["e"], "%s: %v", version, entries)
		assert.Contains(t, entries, manifestEntry{"e.in", "./sub", filepath.Join(ext, "sub")}, version)
	})
}

const manifestOutput = "output \"x\" {\n  value = 1\n}\n"

// manifestEntry is one modules.json entry.
type manifestEntry struct{ Key, Source, Dir string }

// realTempDir is a temp dir with its symlinks resolved, so it compares
// equal to the absolute paths tofu writes (macOS's /var is a symlink).
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

// manifestInit writes files under tmp, runs the first init in tmp/root
// with TF_DATA_DIR at tmp/data and both proxies at the closed port, and
// returns the manifest's entries (nil when tofu wrote none) and the tofu
// version.
func manifestInit(t *testing.T, tmp string, files map[string]string) ([]manifestEntry, string) {
	t.Helper()
	w := assumptionDir{dir: filepath.Join(tmp, "root")}
	for name, content := range files {
		e2e.WriteFile(t, filepath.Join(tmp, name), []byte(content))
	}
	data := filepath.Join(tmp, "data")
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "TF_DATA_DIR=" + data,
		"HTTPS_PROXY=" + closedEndpoint, "HTTP_PROXY=" + closedEndpoint}
	version, err := w.tofu(t, env, "version")
	require.NoError(t, err, version)
	w.version = strings.TrimSpace(version)
	out, err := w.tofu(t, env, "init", "-backend=false", "-input=false", "-no-color")
	require.NoError(t, err, "%s\n%s", w.version, out)

	raw, err := os.ReadFile(filepath.Join(data, "modules", "modules.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, w.version
	}
	require.NoError(t, err, w.version)
	var manifest struct{ Modules []manifestEntry }
	require.NoError(t, json.Unmarshal(raw, &manifest), "%s: %s", w.version, raw)
	require.NotNil(t, manifest.Modules, "%s: %s", w.version, raw)
	return manifest.Modules, w.version
}

// registryModule is the third-party dependency the init test fetches: a
// public-registry module, pinned to an exact version, with no provider of
// its own.
const (
	registryModule        = "cloudposse/label/null"
	registryModuleVersion = "0.25.0"
)

// TestAssumption_SecondInitMakesNoProxiedConnection: `tofu init
// -backend=false` under an environment of PATH and an empty HOME, with no
// TF_CLI_CONFIG_FILE, TF_PLUGIN_CACHE_DIR or AWS_*, fetches the provider
// and both modules into a TF_DATA_DIR outside the copy. The second init,
// with the backend override, `-reconfigure -get=false -lockfile=readonly
// -plugin-dir=<TF_DATA_DIR>/providers`, then makes no proxied connection;
// without -plugin-dir it does. A direct dial is beyond what the recorder
// can observe, so the claim is "no proxied connection".
//
// Relied on by: rehearsal-prepare-gate, which runs the second init under
// the dead proxy (HLD § Prepare: two work directories, gated before any
// tofu; E3). ADR-0028.
func TestAssumption_SecondInitMakesNoProxiedConnection(t *testing.T) {
	e2e.SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")

	c := newInitCopy(t)
	var location string
	for _, line := range strings.Split(c.firstInit, "\n") {
		if strings.Contains(line, registryModule) && strings.Contains(line, " is available at ") {
			location = line
		}
	}
	if location == "" {
		location = "no trace line gives the registry module's download location"
	}
	t.Logf("%s: %s", c.version, location)
	assert.DirExists(t, filepath.Join(c.data, "providers", "registry.opentofu.org", "hashicorp", "aws", harness.AWSProviderVersion), c.version)
	assert.Subset(t, c.moduleKeys(t), []string{"child", "registry"}, c.version)
	assert.FileExists(t, filepath.Join(c.dir, ".terraform.lock.hcl"), c.version)

	proxy := startRecorder(t)
	out, err := c.secondInit(t, proxy, "-plugin-dir="+filepath.Join(c.data, "providers"))
	assert.NoError(t, err, "%s\n%s", c.version, out)
	assert.Zero(t, proxy.count(), "%s: the second init made a proxied connection: %v", c.version, proxy.requests())

	t.Run("control without -plugin-dir", func(t *testing.T) {
		sibling := newInitCopy(t)
		proxy := startRecorder(t)
		out, _ := sibling.secondInit(t, proxy)
		assert.Positive(t, proxy.count(), "%s: no proxied connection without -plugin-dir\n%s", sibling.version, out)
		t.Logf("%s: the proxy saw %v", sibling.version, proxy.requests())
	})
}

// initCopy is a copy of a root using the aws provider, a local module and
// registryModule, after the rehearsal's first init. Its TF_DATA_DIR and
// its state are outside it.
type initCopy struct {
	assumptionDir
	data, outside string
	// firstInit is the first init's output, at TF_LOG=trace.
	firstInit string
}

func newInitCopy(t *testing.T) initCopy {
	t.Helper()
	tmp := t.TempDir()
	c := initCopy{
		assumptionDir: assumptionDir{dir: filepath.Join(tmp, "copy")},
		data:          filepath.Join(tmp, "data"),
		outside:       filepath.Join(tmp, "outside", "terraform.tfstate"),
	}
	c.write(t, "main.tf", fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = %q
    }
  }

  backend "s3" {
    bucket = "never-reached"
    key    = "terraform.tfstate"
    region = %q
  }
}

module "child" {
  source = "./child"
}

module "registry" {
  source  = %q
  version = %q
}
`, harness.AWSProviderVersion, assumptionRegion, registryModule, registryModuleVersion))
	c.write(t, "child/main.tf", "output \"x\" {\n  value = 1\n}\n")
	version, err := c.tofu(t, c.bareEnv(t), "version")
	require.NoError(t, err, version)
	c.version = strings.TrimSpace(version)

	c.firstInit, err = c.tofu(t, c.bareEnv(t, "TF_LOG=trace"), "init", "-backend=false", "-input=false", "-no-color")
	require.NoError(t, err, "%s: the first init failed\n%s", c.version, c.firstInit)
	return c
}

// bareEnv is the whole of a tofu child's environment: PATH, an empty HOME
// and the TF_DATA_DIR, plus extra.
func (c initCopy) bareEnv(t *testing.T, extra ...string) []string {
	t.Helper()
	return append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"TF_DATA_DIR=" + c.data,
	}, extra...)
}

// moduleKeys is the module keys modules.json names.
func (c initCopy) moduleKeys(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(c.data, "modules", "modules.json"))
	require.NoError(t, err, c.version)
	var manifest struct {
		Modules []struct{ Key string }
	}
	require.NoError(t, json.Unmarshal(raw, &manifest), c.version)
	keys := make([]string, 0, len(manifest.Modules))
	for _, m := range manifest.Modules {
		keys = append(keys, m.Key)
	}
	return keys
}

// secondInit adds the backend override and runs the rehearsal's second
// init, plus flags, with both proxies at proxy and no NO_PROXY.
func (c initCopy) secondInit(t *testing.T, proxy *recorder, flags ...string) (string, error) {
	t.Helper()
	c.write(t, cli.OurOverride, cli.LocalBackendOverride)
	args := append([]string{"init", "-input=false", "-no-color", "-reconfigure", "-get=false",
		"-lockfile=readonly", "-backend-config=path=" + c.outside}, flags...)
	return c.tofu(t, c.bareEnv(t, "HTTPS_PROXY="+proxy.url, "HTTP_PROXY="+proxy.url), args...)
}

// missingVPC is an id no fakeaws VPC has, looked up by id (VpcId.1).
// A filter that matches nothing would not do: fakeaws ignores
// DescribeVpcs filters and lists every VPC.
const missingVPC = "vpc-0000000000000dead"

// TestAssumption_RefreshOnlyUnreadableDataSourceExitsOne: against fakeaws,
// `plan -refresh-only -detailed-exitcode` exits 0 on settled state and 2
// on drift, and 1, naming it, on a data source the mock cannot read.
// `apply -refresh-only` has no exit 2: it exits 0 on drift and 1 on that
// data source.
//
// Relied on by: rehearsal-against-the-copy (HLD § Load and check:
// fakeaws owns the mapping, the refresh-only check; E1). ADR-0028,
// ADR-0032.
func TestAssumption_RefreshOnlyUnreadableDataSourceExitsOne(t *testing.T) {
	w, fakeaws, _ := newFakeawsDir(t)
	proxy := startRecorder(t)
	env := w.env(t, proxy)
	w.write(t, "main.tf", providerBlock("")+`resource "aws_vpc" "v" {
  cidr_block = "10.0.0.0/16"
}

output "id" {
  value = aws_vpc.v.id
}
`)
	out, err := w.tofu(t, env, "apply", "-auto-approve", "-input=false", "-no-color")
	require.NoError(t, err, "%s\n%s", w.version, out)
	out, err = w.tofu(t, env, "apply", "-refresh-only", "-auto-approve", "-input=false", "-no-color")
	require.NoError(t, err, "%s: settling the state\n%s", w.version, out)
	out, err = w.tofu(t, env, "plan", "-refresh-only", "-detailed-exitcode", "-input=false", "-no-color")
	require.Equal(t, 0, exitCode(err), "%s: settled state\n%s", w.version, out)

	id, err := w.tofu(t, env, "output", "-raw", "id")
	require.NoError(t, err, "%s\n%s", w.version, id)
	resp, err := http.PostForm(fakeaws.URL+fakeawsPath("EC2"), url.Values{
		"Action": {"CreateTags"}, "ResourceId.1": {id}, "Tag.1.Key": {"drift"}, "Tag.1.Value": {"yes"},
	})
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "CreateTags on %s", id)

	out, err = w.tofu(t, env, "plan", "-refresh-only", "-detailed-exitcode", "-input=false", "-no-color", "-out", "p")
	assert.Equal(t, 2, exitCode(err), "%s: drift\n%s", w.version, out)
	out, err = w.tofu(t, env, "show", "-json", "p")
	require.NoError(t, err, "%s\n%s", w.version, out)
	var plan struct {
		ResourceDrift []struct {
			Address string `json:"address"`
			Change  struct {
				After struct {
					Tags map[string]string `json:"tags"`
				} `json:"after"`
			} `json:"change"`
		} `json:"resource_drift"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &plan), w.version)
	require.Len(t, plan.ResourceDrift, 1, "%s\n%s", w.version, out)
	assert.Equal(t, "aws_vpc.v", plan.ResourceDrift[0].Address, w.version)
	assert.Equal(t, "yes", plan.ResourceDrift[0].Change.After.Tags["drift"], w.version)

	w.write(t, "missing.tf", fmt.Sprintf("data \"aws_vpc\" \"missing\" {\n  id = %q\n}\n", missingVPC))
	out, err = w.tofu(t, env, "plan", "-refresh-only", "-detailed-exitcode", "-input=false", "-no-color")
	assert.Equal(t, 1, exitCode(err), "%s: an unreadable data source\n%s", w.version, out)
	assert.Contains(t, out, "data.aws_vpc.missing", w.version)
	out, err = w.tofu(t, env, "apply", "-refresh-only", "-auto-approve", "-input=false", "-no-color")
	assert.Equal(t, 1, exitCode(err), "%s: apply with an unreadable data source\n%s", w.version, out)
	assert.Contains(t, out, "data.aws_vpc.missing", w.version)
	assert.NotContains(t, w.stateTags(t, env, "aws_vpc.v"), "drift", "%s: the failed apply wrote the drift", w.version)

	require.NoError(t, os.Remove(filepath.Join(w.dir, "missing.tf")))
	out, err = w.tofu(t, env, "apply", "-refresh-only", "-auto-approve", "-input=false", "-no-color")
	assert.Equal(t, 0, exitCode(err), "%s: apply with drift alone\n%s", w.version, out)
	assert.Equal(t, "yes", w.stateTags(t, env, "aws_vpc.v")["drift"], "%s: the drift is not in the state", w.version)
	assert.Zero(t, proxy.count(), "%s: requests left loopback: %v", w.version, proxy.requests())
}

// stateTags is the tags of the resource at address in the state, from
// `tofu show -json`'s stdout.
func (w assumptionDir) stateTags(t *testing.T, env []string, address string) map[string]string {
	t.Helper()
	stdout, stderr, err := cli.RunTofu(w.dir, env, "show", "-json")
	require.NoError(t, err, "%s\n%s%s", w.version, stdout, stderr)
	var state struct {
		Values struct {
			RootModule struct {
				Resources []struct {
					Address string `json:"address"`
					Values  struct {
						Tags map[string]string `json:"tags"`
					} `json:"values"`
				} `json:"resources"`
			} `json:"root_module"`
		} `json:"values"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &state), w.version)
	for _, r := range state.Values.RootModule.Resources {
		if r.Address == address {
			return r.Values.Tags
		}
	}
	require.Failf(t, "not in the state", "%s: %s\n%s", w.version, address, stdout)
	return nil
}

// lookups is more independent data sources than tofu's default
// -parallelism of 10; lookupID formats each one's id, in Go and in HCL.
const (
	lookups  = 16
	lookupID = "vpc-%017d"
)

// TestAssumption_DataSourcesAllSendBeforePlanFails: against an empty
// fakeaws, where every lookup fails, `plan` still sends the DescribeVpcs
// of each of more independent data sources than tofu's -parallelism
// before it exits 1.
//
// Relied on by: rehearsal-against-the-copy (HLD § Discover: let the code
// say what it reads). ADR-0028, ADR-0032.
func TestAssumption_DataSourcesAllSendBeforePlanFails(t *testing.T) {
	w, _, calls := newFakeawsDir(t)
	proxy := startRecorder(t)
	w.write(t, "main.tf", providerBlock("")+fmt.Sprintf(`data "aws_vpc" "lookup" {
  count = %d
  id    = format(%q, count.index)
}
`, lookups, lookupID))

	out, err := w.tofu(t, w.env(t, proxy), "plan", "-input=false", "-no-color")
	assert.Equal(t, 1, exitCode(err), "%s\n%s", w.version, out)
	want, got := map[string]int{}, map[string]int{}
	for i := range lookups {
		want[fmt.Sprintf(lookupID, i)] = 1
	}
	for _, c := range calls.all() {
		if c.service == "ec2" && c.action == "DescribeVpcs" {
			got[c.form.Get("VpcId.1")]++
		}
	}
	assert.Equal(t, want, got, "%s: one DescribeVpcs per index\n%s", w.version, out)
	assert.Zero(t, proxy.count(), "%s: requests left loopback: %v", w.version, proxy.requests())
}

// newFakeawsDir starts a fresh fakeaws before SealNetwork, and a
// recording proxy in front of it, and returns an assumptionDir whose
// override sends EC2 and STS through the proxy at fakeaws's paths.
func newFakeawsDir(t *testing.T) (assumptionDir, *e2e.MockwayInstance, *awsRecorder) {
	t.Helper()
	e2e.SkipUnlessEnabled(t)
	fakeaws := e2e.StartFakeaws(t)
	calls := startAWSRecorder(t, fakeaws.URL)
	w := newAssumptionDir(t)
	var b strings.Builder
	b.WriteString("provider \"aws\" {\n  endpoints {\n")
	for _, svc := range []string{"EC2", "STS"} {
		fmt.Fprintf(&b, "    %s = %q\n", strings.ToLower(svc), calls.url+fakeawsPath(svc))
	}
	b.WriteString("  }\n}\n")
	w.write(t, cli.OurOverride, b.String())
	return w, fakeaws, calls
}

// fakeawsPath is svc's fakeaws path in assumptionRegion.
func fakeawsPath(svc string) string {
	return strings.ReplaceAll(cli.AWSFakeawsPaths[svc], "{region}", assumptionRegion)
}

// exitCode is tofu's exit status: 0 for nil, -1 when it did not exit.
func exitCode(err error) int {
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.ExitCode()
	}
	return -1
}

// awsCall is one request the recorder passed on: the service (the first
// path segment), the Action (from the form body, or X-Amz-Target) and the
// form.
type awsCall struct {
	service, action string
	form            url.Values
}

// awsRecorder is a reverse proxy that keeps the path, records each
// request and passes it on to fakeaws, whose audit table is empty at the
// pin.
type awsRecorder struct {
	url string

	mu    sync.Mutex
	calls []awsCall
}

func startAWSRecorder(t *testing.T, target string) *awsRecorder {
	t.Helper()
	u, err := url.Parse(target)
	require.NoError(t, err)
	next := httputil.NewSingleHostReverseProxy(u)
	r := &awsRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		form, _ := url.ParseQuery(string(body))
		c := awsCall{action: form.Get("Action"), form: form}
		if target := req.Header.Get("X-Amz-Target"); target != "" {
			c.action = target
		}
		c.service, _, _ = strings.Cut(strings.TrimPrefix(req.URL.Path, "/"), "/")
		r.mu.Lock()
		r.calls = append(r.calls, c)
		r.mu.Unlock()
		next.ServeHTTP(w, req)
	}))
	t.Cleanup(server.Close)
	r.url = server.URL
	return r
}

func (r *awsRecorder) all() []awsCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

// assumptionDir is a tofu work directory, initialised from SealNetwork's
// mirror, with the tofu version every failure names.
type assumptionDir struct {
	dir     string
	version string
}

func newAssumptionDir(t *testing.T) assumptionDir {
	t.Helper()
	e2e.SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	t.Cleanup(e2e.CleanupAWSMirror)
	e2e.SealNetwork(t)

	conn, err := net.DialTimeout("tcp", closedAddr, time.Second)
	if err == nil {
		_ = conn.Close()
	}
	require.ErrorIs(t, err, syscall.ECONNREFUSED, "%s must refuse a connection: the tests rely on it being closed", closedAddr)

	w := assumptionDir{dir: t.TempDir()}
	version, err := w.tofu(t, w.env(t, startRecorder(t)), "version")
	require.NoError(t, err, version)
	w.version = strings.TrimSpace(version)
	w.write(t, "versions.tf", fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = %q
    }
  }
}
`, harness.AWSProviderVersion))
	w.init(t)
	return w
}

func (w assumptionDir) write(t *testing.T, name, content string) {
	t.Helper()
	e2e.WriteFile(t, filepath.Join(w.dir, name), []byte(content))
}

func (w assumptionDir) init(t *testing.T) {
	t.Helper()
	out, err := w.tofu(t, w.env(t, startRecorder(t)), "init", "-input=false", "-no-color")
	require.NoError(t, err, "%s\n%s", w.version, out)
}

// env is the whole of a tofu child's environment, never this process's, so
// nothing in the shell (a real key, a profile, an endpoint) reaches it.
func (w assumptionDir) env(t *testing.T, proxy *recorder, extra ...string) []string {
	t.Helper()
	cliConfig := os.Getenv("TF_CLI_CONFIG_FILE")
	require.NotEmpty(t, cliConfig, "SealNetwork sets TF_CLI_CONFIG_FILE")
	return append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + t.TempDir(),
		"TF_CLI_CONFIG_FILE=" + cliConfig,
		"HTTPS_PROXY=" + proxy.url,
		"HTTP_PROXY=" + proxy.url,
		"AWS_REGION=" + assumptionRegion,
		"AWS_ACCESS_KEY_ID=" + fakeAccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + fakeSecretAccessKey,
	}, extra...)
}

// tofu runs tofu in the directory through cli.RunTofu and returns its
// stdout and stderr together; running past the deadline fails the test.
func (w assumptionDir) tofu(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()
	stdout, stderr, err := cli.RunTofu(w.dir, env, args...)
	out := stdout + stderr
	require.NotErrorIs(t, err, context.DeadlineExceeded, "%s\n%s", w.version, out)
	return out, err
}

// endpointKeys is every key of the provider's `endpoints` block, from
// `tofu providers schema -json`.
func (w assumptionDir) endpointKeys(t *testing.T) []string {
	t.Helper()
	out, err := w.tofu(t, w.env(t, startRecorder(t)), "providers", "schema", "-json")
	require.NoError(t, err, "%s\n%s", w.version, out)
	var schema struct {
		ProviderSchemas map[string]struct {
			Provider struct {
				Block struct {
					BlockTypes map[string]struct {
						Block struct {
							Attributes map[string]json.RawMessage `json:"attributes"`
						} `json:"block"`
					} `json:"block_types"`
				} `json:"block"`
			} `json:"provider"`
		} `json:"provider_schemas"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &schema), w.version)
	attributes := schema.ProviderSchemas["registry.opentofu.org/hashicorp/aws"].Provider.Block.BlockTypes["endpoints"].Block.Attributes
	require.NotEmpty(t, attributes, "%s: no endpoints block in the aws provider schema", w.version)
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// applyJSON runs `tofu apply -json`, which must fail, and returns its
// error diagnostics and its alias warnings. Naming every key draws an
// "Only one of the following attributes should be set" warning per pair of
// aliases of one service; the human output folds them into one, so they
// are counted here, logged, and fail the test if they become errors.
func (w assumptionDir) applyJSON(t *testing.T, proxy *recorder) (string, int) {
	t.Helper()
	out, err := w.tofu(t, w.env(t, proxy), "apply", "-auto-approve", "-input=false", "-json")
	require.Error(t, err, "%s: apply succeeded\n%s", w.version, out)
	var errs []string
	aliasWarnings := 0
	for _, line := range strings.Split(out, "\n") {
		var message struct {
			Diagnostic *struct{ Severity, Summary, Detail string } `json:"diagnostic"`
		}
		if json.Unmarshal([]byte(line), &message) != nil || message.Diagnostic == nil {
			continue
		}
		d := message.Diagnostic
		alias := strings.HasPrefix(d.Detail, aliasConflict)
		switch {
		case alias && d.Severity == "warning":
			aliasWarnings++
		case alias:
			t.Errorf("%s: an alias conflict is now %s: %s", w.version, d.Severity, d.Detail)
		case d.Severity == "error":
			errs = append(errs, d.Summary)
		}
	}
	t.Logf("%s: %d %q warnings", w.version, aliasWarnings, aliasConflict)
	return strings.Join(errs, "\n"), aliasWarnings
}

// providerBlock is `provider "aws"` with max_retries = 1 and extra.
func providerBlock(extra string) string {
	return "provider \"aws\" {\n  max_retries = 1\n" + extra + "}\n\n"
}

// endpointsOverride is a `provider "aws"` override routing each key to url.
func endpointsOverride(keys []string, url string) string {
	var b strings.Builder
	b.WriteString("provider \"aws\" {\n  endpoints {\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "    %s = %q\n", key, url)
	}
	b.WriteString("  }\n}\n")
	return b.String()
}

// countEndpoints is how many keys an override's endpoints block names.
func countEndpoints(t *testing.T, override string) int {
	t.Helper()
	f, diags := hclsyntax.ParseConfig([]byte(override), cli.OurOverride, hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())
	for _, provider := range f.Body.(*hclsyntax.Body).Blocks {
		for _, block := range provider.Body.Blocks {
			if block.Type == "endpoints" {
				return len(block.Body.Attributes)
			}
		}
	}
	return 0
}

func without(keys []string, drop string) []string {
	return slices.DeleteFunc(slices.Clone(keys), func(key string) bool { return key == drop })
}

// recorder is a loopback listener that counts the connections it accepts
// and logs each one's first request line: `CONNECT <host>` when it is a
// proxy, `<method> <host> <Action>` when it is an endpoint. It answers
// nothing, so the request fails after it is counted.
type recorder struct {
	url string

	mu       sync.Mutex
	accepted int
	lines    []string
}

func startRecorder(t *testing.T) *recorder {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	r := &recorder{url: "http://" + listener.Addr().String()}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go r.record(conn)
		}
	}()
	return r
}

func (r *recorder) record(conn net.Conn) {
	defer conn.Close()
	r.mu.Lock()
	r.accepted++
	r.mu.Unlock()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	line := req.Method + " " + req.Host
	if req.Method != http.MethodConnect {
		line += " " + req.PostFormValue("Action")
	}
	r.mu.Lock()
	r.lines = append(r.lines, line)
	r.mu.Unlock()
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.accepted
}

func (r *recorder) requests() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.lines)
}
