package cli_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
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
	// tofuDeadline bounds every tofu call. max_retries = 1 in each
	// fixture keeps a refused request fast; without it the SDK retries
	// for more than ten minutes.
	tofuDeadline = 2 * time.Minute
	// ourOverride is the name infrafactory's override takes: it sorts
	// before a user's zz_override.tf.
	ourOverride = "infrafactory_override.tf"
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
		w.write(t, ourOverride, override)

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
		w.write(t, ourOverride, override)

		errs, aliasWarnings := w.applyJSON(t, proxy)
		assert.Positive(t, aliasWarnings, "%s: no %q warning: reworded, or now an error?", w.version, aliasConflict)
		assert.Contains(t, errs, "DescribeVpcs", w.version)
		assert.Contains(t, errs, closedDial, "%s: %s", w.version, errs)
		assert.Zero(t, proxy.count(), "%s: requests left loopback: %v", w.version, proxy.requests())
	})

	t.Run("control without sts", func(t *testing.T) {
		proxy := startRecorder(t)
		w.write(t, "main.tf", providerBlock("")+`data "aws_caller_identity" "me" {}`+"\n")
		w.write(t, ourOverride, endpointsOverride(without(keys, "sts"), closedEndpoint))

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
	w.write(t, ourOverride, endpointsOverride([]string{"sts"}, closedEndpoint))
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
	w.write(t, ourOverride, endpointsOverride([]string{"sts"}, root.url))
	w.write(t, "child/main.tf", providerBlock("")+`data "aws_caller_identity" "me" {}`+"\n")
	w.init(t)

	out, _ := w.tofu(t, w.env(t, proxy, "AWS_ENDPOINT_URL_STS="+child.url),
		"apply", "-auto-approve", "-input=false", "-no-color")
	assert.Positive(t, child.count(), "%s: the child's STS request went elsewhere\n%s", w.version, out)
	assert.Zero(t, root.count(), "%s: the root override reached the child: %v", w.version, root.requests())
	assert.Zero(t, proxy.count(), "%s: requests left loopback: %v", w.version, proxy.requests())
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
	assert.NotEmpty(t, location, "%s: no trace line gives the registry module's download location: reworded?", c.version)
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
		assert.Positive(t, proxy.count(), "%s: no proxied connection without -plugin-dir\n%s", c.version, out)
		t.Logf("%s: the proxy saw %v", c.version, proxy.requests())
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
	c.write(t, ourOverride, cli.LocalBackendOverride)
	args := append([]string{"init", "-input=false", "-no-color", "-reconfigure", "-get=false",
		"-lockfile=readonly", "-backend-config=path=" + c.outside}, flags...)
	return c.tofu(t, c.bareEnv(t, "HTTPS_PROXY="+proxy.url, "HTTP_PROXY="+proxy.url), args...)
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

// tofu runs tofu in the directory under tofuDeadline and returns its
// combined output. On the deadline it SIGKILLs tofu's whole process group:
// a child that outlived a kill of tofu alone would hold the output pipe
// and block the call past the deadline. Killing the group closes every
// holder of the pipe, so there is no WaitDelay, whose timer would also
// start on a normal exit and turn a slow pipe close into exec.ErrWaitDelay
// (exec_runner.go refuses it for the same reason).
func (w assumptionDir) tofu(t *testing.T, env []string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), tofuDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tofu", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.Dir = w.dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err(), "%s: tofu %s ran past %s\n%s", w.version, args, tofuDeadline, out)
	return string(out), err
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
	f, diags := hclsyntax.ParseConfig([]byte(override), ourOverride, hcl.Pos{Line: 1, Column: 1})
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
