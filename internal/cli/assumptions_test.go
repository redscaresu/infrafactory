package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// encryptionProbe is one copy of a root whose state is encrypted by an
// `external` key provider: a script that touches marker when tofu runs
// it. Its tofu children see only PATH and an empty HOME.
type encryptionProbe struct {
	dir, marker, outside, version string
	env                           []string
}

// encryptionProbeKey is the 32-byte AES key the script hands tofu, base64.
var encryptionProbeKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x2a}, 32))

// newEncryptionProbe skips without tofu 1.10 or later (the external key
// provider's release), then writes the script and the root. The root
// declares a remote backend, as the user's does: the first init skips
// it and the second's override replaces it.
func newEncryptionProbe(t *testing.T) encryptionProbe {
	t.Helper()
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Skip("tofu is not on PATH")
	}
	tmp := t.TempDir()
	p := encryptionProbe{
		dir:     filepath.Join(tmp, "copy"),
		marker:  filepath.Join(tmp, "marker"),
		outside: filepath.Join(tmp, "outside", "terraform.tfstate"),
		env:     []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()},
	}
	require.NoError(t, os.Mkdir(p.dir, 0o700))
	p.version = p.tofuVersion(t)

	script := filepath.Join(tmp, "key-provider.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
touch '`+p.marker+`'
echo '{"magic":"OpenTofu-External-Key-Provider","version":1}'
cat >/dev/null
echo '{"keys":{"encryption_key":"`+encryptionProbeKey+`","decryption_key":"`+encryptionProbeKey+`"}}'
`), 0o700))

	require.NoError(t, os.WriteFile(filepath.Join(p.dir, "main.tf"), []byte(`terraform {
  backend "s3" {
    bucket = "never-reached"
    key    = "terraform.tfstate"
    region = "eu-west-1"
  }

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
}

resource "terraform_data" "probe" {}

output "id" {
  value = terraform_data.probe.id
}
`), 0o600))
	return p
}

func (p encryptionProbe) tofuVersion(t *testing.T) string {
	t.Helper()
	out, err := p.tofu("version", "-json")
	require.NoError(t, err)
	var v struct {
		Version string `json:"terraform_version"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &v), out)
	var major, minor int
	_, err = fmt.Sscanf(v.Version, "%d.%d", &major, &minor)
	require.NoError(t, err, "tofu version %q", v.Version)
	if major < 1 || (major == 1 && minor < 10) {
		t.Skipf("tofu %s is below 1.10, which added the external key provider", v.Version)
	}
	return "tofu " + v.Version
}

// encryptionProbeDeadline bounds each tofu command. The key provider
// script waits for stdin to close, so a tofu that holds it open would
// otherwise hang the test until go test's own timeout, naming nothing.
const encryptionProbeDeadline = 2 * time.Minute

// tofu runs tofu in the copy and returns its stdout; a non-zero exit or
// the deadline returns an error carrying the tofu version and both streams.
func (p encryptionProbe) tofu(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), encryptionProbeDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tofu", args...)
	cmd.Dir, cmd.Env = p.dir, p.env
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("killed after %s: %w", encryptionProbeDeadline, ctx.Err())
	}
	if err != nil {
		return string(out), fmt.Errorf("%s: tofu %s: %w\n%s%s", p.version, strings.Join(args, " "), err, out, stderr.String())
	}
	return string(out), nil
}

// initWithOutsideBackend is the rehearsal's second init: a `backend
// "local" {}` override with no path, given one outside the copy.
func (p encryptionProbe) initWithOutsideBackend() error {
	override := "terraform {\n  backend \"local\" {}\n}\n"
	if err := os.WriteFile(filepath.Join(p.dir, "override.tf"), []byte(override), 0o600); err != nil {
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

	require.NoError(t, p.initWithOutsideBackend(), p.version)
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
	require.NoError(t, p.initWithOutsideBackend(), p.version)
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
