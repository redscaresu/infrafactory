package cli_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/cli"
	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/e2e"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// TestE2E_AWSLayer2SeedsTheResolvedAMI runs an aws Layer 3 `test` up to the
// real apply, on fakeaws, for an AL2023-shaped id fakeaws has no fixture
// for: the step-one stack naming it passes the AWS gate, and the Layer 2
// mock deploy applies that same stack, because the deploy seeds the id
// after its reset. The fixture's id is refused by the gate, so the gate
// ran against the resolved one. It lives here, not in internal/e2e,
// because only this package's tests can set the resolved AMI; the
// required CI job runs it by name.
func TestE2E_AWSLayer2SeedsTheResolvedAMI(t *testing.T) {
	e2e.SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	mock := e2e.StartFakeaws(t)
	// The mirror outlives a test; internal/e2e's TestMain removes it there.
	t.Cleanup(e2e.CleanupAWSMirror)
	e2e.SealNetwork(t)

	const resolved = "ami-0123456789abcdef0"
	root := harness.AWSAMIRoot{DeviceName: "/dev/xvda", SizeGiB: 8, VolumeType: "gp3", DeleteOnTermination: true}
	fixtures := filepath.Join(e2e.RepoRoot(t), "internal", "e2e", "testdata", "aws-web-step-one")
	hcl, err := os.ReadFile(filepath.Join(fixtures, "web-step-one.tf"))
	require.NoError(t, err)
	cfg := config.Default()
	cfg.Fakeaws.URL = mock.URL
	cfg.AWS.Region = "us-east-1"
	deploy := func(ami string) error {
		main := bytes.ReplaceAll(hcl, []byte(harness.AWSLayer2AMI), []byte(ami))
		return cli.AWSLayer3MockDeploy(context.Background(), cfg, filepath.Join(fixtures, "web-step-one.yaml"),
			map[string][]byte{"main.tf": main}, resolved, root)
	}

	assert.ErrorContains(t, deploy(harness.AWSLayer2AMI), "aws gate:", "the gate admits only the resolved id")
	require.NoError(t, deploy(resolved), "fakeaws log: %s", mock.LogPath())
	ec2, _ := mock.FetchState(t)["ec2"].(map[string]any)
	assert.Len(t, ec2["instances"], 1, "the mock launched the instance")
}
