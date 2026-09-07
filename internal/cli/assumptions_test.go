package cli

import (
	"bytes"
	"errors"
	"io"
	"testing"

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
