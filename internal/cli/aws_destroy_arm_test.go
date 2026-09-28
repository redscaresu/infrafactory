package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// recordingSandboxDestroy records what Run was handed, and counts
// RunWithoutConfig apart from it.
type recordingSandboxDestroy struct {
	err                error
	calls              int
	withoutConfigCalls int
	workDir            string
	env                map[string]string
}

func (r *recordingSandboxDestroy) Run(_ context.Context, workDir string, env map[string]string) (*harness.SandboxDestroyResult, error) {
	r.calls++
	r.workDir, r.env = workDir, env
	if r.err != nil {
		return nil, r.err
	}
	return &harness.SandboxDestroyResult{Destroy: harness.StageResult{Stage: "destroy"}}, nil
}

func (r *recordingSandboxDestroy) RunWithoutConfig(context.Context, string, string, map[string]string) (*harness.SandboxDestroyResult, error) {
	r.withoutConfigCalls++
	return &harness.SandboxDestroyResult{}, nil
}

func awsSealedEnvForTest(t *testing.T) map[string]string {
	t.Helper()
	credFile := filepath.Join(t.TempDir(), "layer3-aws.env")
	require.NoError(t, os.WriteFile(credFile, []byte("AWS_ACCESS_KEY_ID=AKIAFROMTHEFILE00001\nAWS_SECRET_ACCESS_KEY=s\n"), 0o600))
	env, err := harness.AWSSealedEnv(credFile, "eu-west-1")
	require.NoError(t, err)
	return env
}

// awsDestroyFixture is an AWS state beside a stale Scaleway marker, so
// any Scaleway remediation that ran would find a project to act on.
func awsDestroyFixture(t *testing.T, destroy *recordingSandboxDestroy) (*CommandRuntime, layer3Fakes, string) {
	t.Helper()
	workDir := t.TempDir()
	writeAWSStateAndStaleMarker(t, workDir, true)
	fakes := newLayer3Fakes()
	rt := &CommandRuntime{}
	fakes.install(&rt.Deps)
	rt.Deps.SandboxDestroy = destroy
	return rt, fakes, workDir
}

// assertOnlyTheDestroyRan: RunProject and AutoCreated are every Scaleway
// HTTP call a destroy can make.
func assertOnlyTheDestroyRan(t *testing.T, fakes layer3Fakes, destroy *recordingSandboxDestroy, runs int) {
	t.Helper()
	assert.Equal(t, runs, destroy.calls, "SandboxDestroy.Run")
	assert.Zero(t, destroy.withoutConfigCalls, "RunWithoutConfig")
	assert.Zero(t, fakes.runProject.calls, "RunProject")
	assert.Zero(t, fakes.purge.calls, "AutoCreated")
	assert.Zero(t, fakes.sweep.calls, "OrphanSweep")
	assert.Zero(t, fakes.deploy.calls, "SandboxDeploy")
}

func TestAWSDestroyRunsOnceWithTheSealedEnv(t *testing.T) {
	destroy := &recordingSandboxDestroy{}
	rt, fakes, workDir := awsDestroyFixture(t, destroy)
	env := awsSealedEnvForTest(t)

	result, removed, err := destroySandbox(context.Background(), rt, layer3AWS, workDir, env, staleMarkerProjectID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, removed)
	assert.Equal(t, workDir, destroy.workDir)
	assert.Equal(t, env, destroy.env)
	assertOnlyTheDestroyRan(t, fakes, destroy, 1)
}

func TestAWSDestroyFailureIsReturnedWithoutARetryOrFallback(t *testing.T) {
	wantErr := errors.New("tofu destroy failed")
	destroy := &recordingSandboxDestroy{err: wantErr}
	rt, fakes, workDir := awsDestroyFixture(t, destroy)

	_, removed, err := destroySandbox(context.Background(), rt, layer3AWS, workDir, awsSealedEnvForTest(t), staleMarkerProjectID)

	require.ErrorIs(t, err, wantErr)
	assert.Empty(t, removed)
	assertOnlyTheDestroyRan(t, fakes, destroy, 1)

	// The premise: Scaleway's arm, on the same fixture and failure,
	// reaches the Account API, so the zero above is the arm's doing.
	env := awsSealedEnvForTest(t)
	env["SCW_SECRET_KEY"] = "real-secret"
	_, _, _ = destroySandbox(context.Background(), rt, layer3Scaleway, workDir, env, staleMarkerProjectID)
	assert.NotZero(t, fakes.runProject.calls, "RunProject")
}

func TestAWSDestroyRefusesAnEnvThatIsNotSealed(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"nil":                  nil,
		"no AWS_ACCESS_KEY_ID": {"AWS_SECRET_ACCESS_KEY": "s", "AWS_REGION": "eu-west-1", "SCW_SECRET_KEY": "real-secret"},
	} {
		t.Run(name, func(t *testing.T) {
			destroy := &recordingSandboxDestroy{}
			rt, fakes, workDir := awsDestroyFixture(t, destroy)

			_, _, err := destroySandbox(context.Background(), rt, layer3AWS, workDir, env, staleMarkerProjectID)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "AWS_ACCESS_KEY_ID")
			assertOnlyTheDestroyRan(t, fakes, destroy, 0)
		})
	}
}

func TestGCPDestroyStillRefuses(t *testing.T) {
	destroy := &recordingSandboxDestroy{}
	rt, fakes, workDir := awsDestroyFixture(t, destroy)

	_, _, err := destroySandbox(context.Background(), rt, layer3Cloud("gcp"), workDir, awsSealedEnvForTest(t), staleMarkerProjectID)

	require.Error(t, err)
	assert.Equal(t, layer3SeamRefused("gcp", "destroy").Error(), err.Error())
	assertOnlyTheDestroyRan(t, fakes, destroy, 0)
}
