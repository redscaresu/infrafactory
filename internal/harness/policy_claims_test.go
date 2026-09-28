package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deny_state reads the Layer 2 mock's state on every run, including one
// that also applied at Layer 3: criteria evaluation is handed the mock
// deploy's snapshot (pinned by internal/cli/state_policy_mock_only_test.go).
// A comment claiming otherwise reads as real-state coverage that does not
// exist. The required phrase is what a rewording drops; the forbidden ones
// are the ways the false claim has been, or would be, written.
var falseDenyStateClaims = []string{"layer 3", "real state", "actually created", "provider created", "really created"}

func TestDenyStateClaimsNameTheLayer2Mock(t *testing.T) {
	t.Parallel()

	for _, cloud := range []string{"aws", "scaleway"} {
		paths, err := filepath.Glob(filepath.Join("..", "..", "policies", cloud, "*.rego"))
		require.NoError(t, err)
		require.NotEmpty(t, paths, "policies/%s must hold policies, or this ratchet checks nothing", cloud)

		for _, path := range paths {
			defined, err := PolicyFileDefinesRule(path, "deny_state")
			require.NoError(t, err)
			if !defined {
				continue
			}
			t.Run(filepath.Join(cloud, filepath.Base(path)), func(t *testing.T) {
				comments := regoCommentText(t, path)
				// assert.True, not Contains: the haystack is every comment
				// in the file, and dumping it buries the one-line reason.
				assert.True(t, strings.Contains(comments, "layer 2 mock"),
					"%s defines deny_state and must say, in a comment, that it reads the Layer 2 mock's state", path)
				for _, phrase := range falseDenyStateClaims {
					assert.False(t, strings.Contains(comments, phrase),
						"%s defines deny_state; a comment there must not claim %q", path, phrase)
				}
			})
		}
	}

	t.Run("ADR-0034 §4", func(t *testing.T) {
		section := adrSection(t, "../../docs/decisions/0034-a-prohibition-is-a-specification.md", "### 4.")
		assert.Contains(t, section, "Layer 2 mock")
		assert.Contains(t, section, "holdout")
	})
}

// regoCommentText is every comment in the file, lower-cased and joined
// with single spaces, so a phrase wrapped across two comment lines still
// matches.
func regoCommentText(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	require.NoError(t, err)
	module, err := ast.ParseModule(path, string(payload))
	require.NoError(t, err)

	words := make([]string, 0)
	for _, c := range module.Comments {
		words = append(words, strings.Fields(strings.ToLower(string(c.Text)))...)
	}
	return strings.Join(words, " ")
}

// adrSection is the text from the heading that starts with prefix up to
// the next heading of the same level.
func adrSection(t *testing.T, path, prefix string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	require.NoError(t, err)

	_, rest, found := strings.Cut(string(payload), "\n"+prefix)
	require.True(t, found, "%s has no %q heading", path, prefix)
	section, _, _ := strings.Cut(rest, "\n### ")
	section, _, _ = strings.Cut(section, "\n## ")
	return section
}
