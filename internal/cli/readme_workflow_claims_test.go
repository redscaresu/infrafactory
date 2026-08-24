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

	// The disclaimer is what makes the absence honest.
	disclaimed := strings.Contains(string(readme), "not yet merged")

	if gateExists {
		assert.False(t, disclaimed,
			"the layer3-gate workflow exists now, so the README must stop saying the gate is not yet merged")
		return
	}
	assert.True(t, disclaimed,
		"there is no .github/workflows/layer3-gate.yml, so the README must not present the "+
			"pre-merge gate as something this repository enforces")
}
