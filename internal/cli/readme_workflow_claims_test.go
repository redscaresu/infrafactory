package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The README must not promise a CI gate the repository does not have.
//
// The Layer 3 pull-request gate was written on a branch and described in
// the README before it merged, which turned a public safety claim into
// something readers could not verify and maintainers would not notice.
// The claim and the workflow file now rise and fall together.
func TestReadmeLayer3GateClaimMatchesWorkflows(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	require.NoError(t, err)

	_, statErr := os.Stat(filepath.Join(root, ".github", "workflows", "layer3-gate.yml"))
	gateExists := statErr == nil

	// Collapse whitespace before matching. Markdown wraps prose at
	// whatever column it likes, so a phrase can be split across a
	// newline and a literal substring check then silently stops
	// matching -- which is a guard that fails for a reason having
	// nothing to do with what it guards.
	text := strings.Join(strings.Fields(string(readme)), " ")

	// The disclaimer is what makes the absence honest.
	disclaimed := strings.Contains(text, "not yet merged")

	if gateExists {
		assert.False(t, disclaimed,
			"the layer3-gate workflow exists now, so the README must stop saying the gate is not yet merged")
		return
	}
	assert.True(t, disclaimed,
		"there is no .github/workflows/layer3-gate.yml, so the README must not present the "+
			"pre-merge gate as something this repository enforces")

	// A qualification buried in one section does not undo an unqualified
	// promise in the summary -- and the summary is what most readers read.
	// The first version of this test checked only for the disclaimer and
	// missed exactly that.
	for _, overstatement := range []string{
		"before the change is merged",
		"before it merges",
		"before it is merged",
	} {
		assert.NotContains(t, text, overstatement,
			"the README promises real-cloud validation %q while no layer3-gate workflow exists", overstatement)
	}
}
