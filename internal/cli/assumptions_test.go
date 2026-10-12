package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// Assumptions this package's COMMENTS depend on.
// See internal/api/assumptions_test.go for why this file exists.

// Cobra prints a returned error on Execute() and NOT when RunE is
// invoked directly.
//
// Relied on by: live_service.go. A comment there claimed the deploy's
// cause "is on the command's stderr"; `runDeployCommand` is called
// directly, so nothing was written and withholding the detail left the
// reader with nothing at all.
func TestAssumption_CobraOnlyPrintsErrorsFromExecute(t *testing.T) {
	newCmd := func(out io.Writer) *cobra.Command {
		cmd := &cobra.Command{
			Use:  "probe",
			RunE: func(*cobra.Command, []string) error { return errors.New("the cause") },
		}
		cmd.SetOut(io.Discard)
		cmd.SetErr(out)
		cmd.SetArgs(nil)
		return cmd
	}

	var direct bytes.Buffer
	cmd := newCmd(&direct)
	require.Error(t, cmd.RunE(cmd, nil), "the error is returned either way")
	assert.Empty(t, direct.String(),
		"a direct RunE call prints NOTHING: the caller owns the error")

	var executed bytes.Buffer
	require.Error(t, newCmd(&executed).Execute())
	assert.Contains(t, executed.String(), "the cause",
		"Execute prints it, which is what the false comment was thinking of")
}

// A nil pointer in an interface is NOT a nil interface, and testify
// hides that.
//
// Relied on by: the S163f call site, which tests the concrete pointer
// before assigning. `stageProgress` guards on `p.out != nil`, so a typed
// nil makes it format every line into a discarder.
func TestAssumption_ATypedNilIsNotANilInterface(t *testing.T) {
	var concrete *bytes.Buffer
	var iface io.Writer = concrete

	assert.True(t, iface != nil,
		"the RAW comparison, which is the one production guards make")
	assert.Nil(t, iface,
		"testify reflects INTO the interface and disagrees -- which is why "+
			"a contract test for this must compare raw")
}

// HCL parses `a.b.c[0].d` as ONE traversal containing a TraverseIndex,
// with no IndexExpr node anywhere.
//
// Relied on by: layer3_hcl_shape.go's undestroyable-expression check.
// The first version looked for IndexExpr and therefore reported clean on
// the exact stack that stranded real infrastructure.
func TestAssumption_HCLIndexesAreTraversalStepsNotIndexExprs(t *testing.T) {
	src := []byte(`x = scaleway_instance_private_nic.web.private_ips[0].address` + "\n")
	f, diags := hclsyntax.ParseConfig(src, "probe.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())

	attr := f.Body.(*hclsyntax.Body).Attributes["x"]
	require.NotNil(t, attr)

	var sawIndexExpr, sawTraverseIndex bool
	_ = hclsyntax.Walk(attr.Expr, assumptionWalker{
		onNode: func(n hclsyntax.Node) {
			if _, ok := n.(*hclsyntax.IndexExpr); ok {
				sawIndexExpr = true
			}
			if e, ok := n.(*hclsyntax.ScopeTraversalExpr); ok {
				for _, step := range e.Traversal {
					if _, ok := step.(hcl.TraverseIndex); ok {
						sawTraverseIndex = true
					}
				}
			}
		},
	})

	assert.False(t, sawIndexExpr, "there is NO IndexExpr in this shape")
	assert.True(t, sawTraverseIndex, "the index is a step inside the traversal")
}

type assumptionWalker struct{ onNode func(hclsyntax.Node) }

func (w assumptionWalker) Enter(n hclsyntax.Node) hcl.Diagnostics { w.onNode(n); return nil }
func (w assumptionWalker) Exit(hclsyntax.Node) hcl.Diagnostics    { return nil }

// hclsyntax.Walk takes the walker BY VALUE, so state must be behind a
// pointer.
//
// Relied on by: the try()-guard counter in layer3_hcl_shape.go, whose
// first version initialised an int field inside Enter and silently reset
// it on every node.
func TestAssumption_HCLWalkCopiesTheWalker(t *testing.T) {
	src := []byte("x = try(a.b[0], 1)\n")
	f, _ := hclsyntax.ParseConfig(src, "probe.tf", hcl.Pos{Line: 1, Column: 1})
	attr := f.Body.(*hclsyntax.Body).Attributes["x"]

	byValue := &countingWalker{}
	_ = hclsyntax.Walk(attr.Expr, *byValue)
	assert.Zero(t, byValue.seen,
		"a value receiver's mutations are lost: this is the trap")

	shared := 0
	_ = hclsyntax.Walk(attr.Expr, countingWalker{shared: &shared})
	assert.Positive(t, shared, "state behind a pointer survives")
}

type countingWalker struct {
	seen   int
	shared *int
}

func (w countingWalker) Enter(hclsyntax.Node) hcl.Diagnostics {
	w.seen++
	if w.shared != nil {
		*w.shared++
	}
	return nil
}
func (w countingWalker) Exit(hclsyntax.Node) hcl.Diagnostics { return nil }

// backendProbe is one copy of a root that declares a remote backend, as
// the user's does, and a terraform_data whose id is an output; outside is
// a state path outside the copy. Its tofu children see only PATH and an
// empty HOME.
type backendProbe struct {
	dir, outside, version string
	env                   []string
}

// newBackendProbe skips without tofu, then writes the root, with
// terraformExtra inside its `terraform {}` block.
func newBackendProbe(t *testing.T, terraformExtra string) backendProbe {
	t.Helper()
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Skip("tofu is not on PATH")
	}
	tmp := t.TempDir()
	p := backendProbe{
		dir:     filepath.Join(tmp, "copy"),
		outside: filepath.Join(tmp, "outside", "terraform.tfstate"),
		env:     []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()},
	}
	require.NoError(t, os.Mkdir(p.dir, 0o700))
	out, err := p.tofu("version", "-json")
	require.NoError(t, err)
	var v struct {
		Version string `json:"terraform_version"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &v), out)
	p.version = "tofu " + v.Version

	require.NoError(t, os.WriteFile(filepath.Join(p.dir, "main.tf"), []byte(`terraform {
  backend "s3" {
    bucket = "never-reached"
    key    = "terraform.tfstate"
    region = "eu-west-1"
  }
`+terraformExtra+`}

resource "terraform_data" "probe" {}

output "id" {
  value = terraform_data.probe.id
}
`), 0o600))
	return p
}

// encryptionProbe is a backendProbe whose state is encrypted by an
// `external` key provider: a script that touches marker when tofu runs it.
type encryptionProbe struct {
	backendProbe
	marker string
}

// encryptionProbeKey is the 32-byte AES key the script hands tofu, base64.
var encryptionProbeKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x2a}, 32))

// newEncryptionProbe skips without tofu 1.10 or later (the external key
// provider's release), then writes the script and the root. The first
// init skips the root's remote backend and the second's override
// replaces it.
func newEncryptionProbe(t *testing.T) encryptionProbe {
	t.Helper()
	tmp := t.TempDir()
	script := filepath.Join(tmp, "key-provider.sh")
	p := encryptionProbe{marker: filepath.Join(tmp, "marker")}
	p.backendProbe = newBackendProbe(t, `
  encryption {
    key_provider "external" "probe" {
      command = ["`+script+`"]
    }
    method "aes_gcm" "probe" {
      keys = key_provider.external.probe
    }
    state {
      method = method.aes_gcm.probe
    }
  }
`)
	var major, minor int
	_, err := fmt.Sscanf(p.version, "tofu %d.%d", &major, &minor)
	require.NoError(t, err, p.version)
	if major < 1 || (major == 1 && minor < 10) {
		t.Skipf("%s is below 1.10, which added the external key provider", p.version)
	}

	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
touch '`+p.marker+`'
echo '{"magic":"OpenTofu-External-Key-Provider","version":1}'
cat >/dev/null
echo '{"keys":{"encryption_key":"`+encryptionProbeKey+`","decryption_key":"`+encryptionProbeKey+`"}}'
`), 0o700))
	return p
}

// tofuDeadline bounds every tofu call in the assumption tests, so a hang
// fails naming the call instead of running into go test's own timeout: an
// external key provider's script waits for stdin to close, and a request
// refused at a dead address retries for more than ten minutes unless the
// fixture sets max_retries = 1, as each one does.
const tofuDeadline = 2 * time.Minute

// ourOverride is the name infrafactory's override takes: it sorts before a
// user's zz_override.tf.
const ourOverride = "infrafactory_override.tf"

// runTofu runs tofu in dir through the production runner in replace mode
// under tofuDeadline, and returns its stdout and stderr. The child's
// environment is exactly env, plus this process's PATH when env names
// none. The tofu that runs is the one on this process's PATH, whatever
// PATH env names. At the deadline tofu's process group gets the runner's
// SIGINT, then SIGKILL after cancelKillFallback, and the error wraps
// context.DeadlineExceeded.
func runTofu(dir string, env []string, args ...string) (stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), tofuDeadline)
	defer cancel()
	vars := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		vars[key] = value
	}
	result, err := execCommandRunner{}.Run(ctx, harness.Command{Name: "tofu", Args: args, Dir: dir, Env: vars, ReplaceEnv: true})
	if ctx.Err() != nil {
		err = fmt.Errorf("tofu %s ran past %s: %w", strings.Join(args, " "), tofuDeadline, ctx.Err())
	}
	return string(result.Stdout), string(result.Stderr), err
}

// tofu runs tofu in the copy and returns its stdout; a non-zero exit or
// the deadline returns an error carrying the tofu version and both streams.
func (p backendProbe) tofu(args ...string) (string, error) {
	out, stderr, err := runTofu(p.dir, p.env, args...)
	if err != nil {
		return out, fmt.Errorf("%s: tofu %s: %w\n%s%s", p.version, strings.Join(args, " "), err, out, stderr)
	}
	return out, nil
}

// localBackendOverride is the rehearsal's backend override: `backend
// "local"`, with blockPath as its path when it is not empty.
func localBackendOverride(blockPath string) string {
	block := "{}"
	if blockPath != "" {
		block = fmt.Sprintf("{\n    path = %q\n  }", blockPath)
	}
	return "terraform {\n  backend \"local\" " + block + "\n}\n"
}

// initWithOutsideBackend is the rehearsal's second init: the backend
// override, given a path outside the copy on the command line.
func (p backendProbe) initWithOutsideBackend(blockPath string) error {
	override := localBackendOverride(blockPath)
	if err := os.WriteFile(filepath.Join(p.dir, ourOverride), []byte(override), 0o600); err != nil {
		return err
	}
	_, err := p.tofu("init", "-input=false", "-no-color", "-reconfigure", "-get=false",
		"-lockfile=readonly", "-backend-config=path="+p.outside)
	return err
}

// An `external` key provider that `terraform { encryption { state {} } }`
// uses runs its command at the init that initialises the backend, and
// not at `init -backend=false`.
//
// Relied on by: rehearsal-prepare-gate, which refuses `encryption` by
// name before the first tofu command, because the second init would
// otherwise run a command of the user's code (ADR-0028; the import HLD's
// "Prepare" section, review-2 E2).
func TestAssumption_ExternalKeyProviderRunsAtBackendInit(t *testing.T) {
	p := newEncryptionProbe(t)

	_, err := p.tofu("init", "-input=false", "-no-color", "-backend=false")
	require.NoError(t, err, p.version)
	assert.NoFileExists(t, p.marker, "%s: init -backend=false ran the key provider", p.version)

	require.NoError(t, p.initWithOutsideBackend(""), p.version)
	assert.FileExists(t, p.marker,
		"%s: the backend-initialising init did not run the key provider", p.version)
}

// `tofu state pull` emits the state decrypted, though the state at rest
// is encrypted.
//
// Relied on by: rehearsal-state-pull, which pulls state through tofu and
// so would hand the rehearsal plaintext secrets of an encrypted state;
// the gate refuses `encryption` for that too (ADR-0028; the import HLD's
// "Prepare" section, review-2 E2).
func TestAssumption_StatePullEmitsDecryptedState(t *testing.T) {
	p := newEncryptionProbe(t)
	require.NoError(t, p.initWithOutsideBackend(""), p.version)
	_, err := p.tofu("apply", "-input=false", "-no-color", "-auto-approve")
	require.NoError(t, err, p.version)

	id, err := p.tofu("output", "-raw", "id")
	require.NoError(t, err, p.version)
	require.NotEmpty(t, id, p.version)

	onDisk, err := os.ReadFile(p.outside)
	require.NoError(t, err, p.version)
	assert.NotContains(t, string(onDisk), id, "%s: the state at rest is plaintext", p.version)

	pulled, err := p.tofu("state", "pull")
	require.NoError(t, err, p.version)
	assert.Contains(t, pulled, id, "%s: state pull did not emit the decrypted state", p.version)
}

// With the rehearsal's `backend "local"` override, `-backend-config=path=`
// puts the state at a path outside the copy, and no file under the copy,
// `.terraform` included, holds it. The command-line path wins over any
// path in the block (verified on tofu 1.12.6): that is the fact the walls
// rely on, so the test gives the block a path inside the copy too, and
// "no path in the block" is incidental.
//
// Relied on by: rehearsal-walls, which keep the state a rehearsal writes
// out of the copy tofu runs in (ADR-0028; the import HLD's "Prepare"
// section).
func TestAssumption_CLIBackendPathKeepsStateOutOfTheCopy(t *testing.T) {
	for name, blockPath := range map[string]string{
		"no path in the block": "",
		"a path in the block":  "in-copy.tfstate",
	} {
		t.Run(name, func(t *testing.T) {
			p := newBackendProbe(t, "")
			require.NoError(t, p.initWithOutsideBackend(blockPath), p.version)
			_, err := p.tofu("apply", "-input=false", "-no-color", "-auto-approve")
			require.NoError(t, err, p.version)
			id, err := p.tofu("output", "-raw", "id")
			require.NoError(t, err, p.version)
			require.NotEmpty(t, id, p.version)

			outside, err := os.ReadFile(p.outside)
			assert.NoError(t, err, p.version)
			assert.Contains(t, string(outside), id, "%s: the state is not at the command-line path", p.version)
			require.NoError(t, filepath.WalkDir(p.dir, func(path string, d fs.DirEntry, err error) error {
				if err != nil || !d.Type().IsRegular() {
					return err
				}
				content, err := os.ReadFile(path)
				if err == nil {
					assert.NotContains(t, string(content), id, "%s: %s holds the state", p.version, path)
				}
				return err
			}))
		})
	}
}
