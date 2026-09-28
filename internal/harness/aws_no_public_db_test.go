package harness

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeaws's /mock/state lists rds.db_instances without publicly_accessible,
// so a deny_state here could never fire and would report a state pass that
// checked nothing. Without one, a criterion naming it reports a skip.
func TestAWSNoPublicDBHasNoDenyState(t *testing.T) {
	t.Parallel()
	defined, err := PolicyFileDefinesRule(filepath.Join(opaTestRepoRoot(t), "policies", "aws", "no_public_db.rego"), "deny_state")
	require.NoError(t, err)
	assert.False(t, defined)
}
