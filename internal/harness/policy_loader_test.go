package harness

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The dir holds a policy and a v1-only _test.rego beside it, the way a cloud's
// policy dir colocates `opa test` files.
var policyLoaderDir = filepath.Join("testdata", "policy-loader")

func TestDiscoverPolicyPackagesSkipsTestFiles(t *testing.T) {
	t.Parallel()

	packages, err := discoverPolicyPackages([]string{policyLoaderDir})
	require.NoError(t, err)
	assert.Equal(t, []string{"loader.policy"}, packages)

	packages, err = discoverPolicyPackages([]string{filepath.Join(policyLoaderDir, "policy_test.rego")})
	require.NoError(t, err)
	assert.Empty(t, packages)
}

func TestEvaluatePlanPoliciesSkipsTestFiles(t *testing.T) {
	t.Parallel()

	failures, err := EvaluatePlanPoliciesWithParams(context.Background(), []byte(`{}`), nil, []string{policyLoaderDir})
	require.NoError(t, err)
	require.Len(t, failures, 1)
	assert.Equal(t, "loader.policy", failures[0].Policy)
	assert.Equal(t, "plan policy evaluated", failures[0].Detail)
}

func TestEvaluateStatePoliciesSkipsTestFiles(t *testing.T) {
	t.Parallel()

	failures, err := EvaluateStatePoliciesWithInput(context.Background(), []byte(`{}`), nil, []string{policyLoaderDir})
	require.NoError(t, err)
	require.Len(t, failures, 1)
	assert.Equal(t, "loader.policy", failures[0].Policy)
	assert.Equal(t, "state policy evaluated", failures[0].Detail)
}
