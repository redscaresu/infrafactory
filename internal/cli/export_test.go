package cli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/spf13/cobra"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// AWSLayer3MockDeploy is `test`'s aws Layer 3 path up to the real apply,
// for the e2e against a real fakeaws (cli_test, which can start one):
// generated, wired as generation wires it, goes through the AWS gate
// against amiID and root, set here as the resolve sets them, then through
// the Layer 2 mock deploy as the runtime wires it. Test code only, since
// nothing in a command may set the resolved AMI but the resolve.
func AWSLayer3MockDeploy(ctx context.Context, cfg config.Config, scenarioPath string, generated map[string][]byte, amiID string, root harness.AWSAMIRoot) error {
	runtime := &CommandRuntime{Config: cfg, scenarioLoader: defaultScenarioLoader, AWSLayer3AMI: amiID, AWSLayer3AMIRoot: root}
	sc, err := runtime.LoadScenario(scenarioPath)
	if err != nil {
		return err
	}
	in, err := layer3TestGateInputs(runtime, layer3AWS, sc)
	if err != nil {
		return err
	}
	files := maps.Clone(generated)
	if err := ensureAwsProviderWiring(files, cfg, ""); err != nil {
		return err
	}
	if err := placeAWSUserData(files, sc.Cloud, in.UserData); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "infrafactory-aws-layer3-mock-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if _, err := writeGeneratedFiles(dir, files, generatedFileWriteModeClean); err != nil {
		return err
	}
	if err := layer3PreflightHCLForCloud(layer3AWS, dir, in); err != nil {
		return fmt.Errorf("aws gate: %w", err)
	}
	deploy := newMockDeployHarness(runtime, newCloudMockStateRouter(runtime, cfg))
	_, err = deploy.Run(ctx, dir, awsLayer2Env(cfg), harness.MockDeployModeClean)
	// With tofu's stderr, as `test` reports it, so a refusal says why.
	if deployErr := (*harness.MockDeployError)(nil); errors.As(err, &deployErr) {
		return fmt.Errorf("%w: %s", err, mockDeployFailureDetail(deployErr))
	}
	return err
}

// The assumption tests' tofu runner, override name, backend override
// (no path), second init args, fakeaws paths, endpoints key reader and
// provider override, for the assumption tests in cli_test; the walls
// environment and the dead proxy it names, for the fail-closed e2e.
var (
	RunTofu                   = runTofu
	LocalBackendOverride      = localBackendOverride("")
	SecondInitArgs            = rehearsalSecondInitArgs
	AWSFakeawsPaths           = awsFakeawsPaths
	RehearsalEndpointKeys     = rehearsalEndpointKeys
	RehearsalProviderOverride = rehearsalProviderOverride
	RehearsalWallsEnv         = rehearsalWallsEnv
	RehearsalDeadProxy        = rehearsalDeadProxy
)

// OurOverride holds the backend override and AWSOverride the provider
// override, each in its own file as ADR-0042 lays them out.
// RehearsalFakeawsModule and RehearsalFakeawsPin are the fakeaws module
// and commit fakeawsAtPin clones and builds.
const (
	OurOverride            = ourOverride
	AWSOverride            = rehearsalAWSOverride
	RehearsalFakeawsModule = rehearsalFakeawsModule
	RehearsalFakeawsPin    = rehearsalFakeawsPin
)

// StopRehearsalFakeaws stops the fakeaws whose process group is pid, as a
// failed prepare does, for stopRecordedFakeaws's cleanup: it returns once
// the group is gone, or fails naming pid and port after the kill fallback.
func StopRehearsalFakeaws(pid, port int) error { return rehearsalFakeaws{PID: pid, Port: port}.stop() }

// StartRehearsalFakeaws starts bin as prepare starts the rehearsal's
// fakeaws, in dir, which it makes, for the teardown e2e's stand-in prod.
// It returns once fakeaws is ready; StopRehearsalFakeaws stops it.
func StartRehearsalFakeaws(bin, dir, account, region string) (pid, port int, err error) {
	f, err := rehearsalStartFakeaws(context.Background(), rehearsalFakeawsOpts{bin: bin, dir: dir, account: account, region: region})
	return f.PID, f.Port, err
}

// RehearseReadRoot is the root command with `rehearse read` sending every
// request through doer, for the teardown e2e's read of a stand-in prod.
func RehearseReadRoot(doer aws.HTTPClient) *cobra.Command { return readRoot(rehearseRead{doer: doer}) }

// EC2ErrorAnswer is EC2's error answer carrying code, for the teardown
// e2e's stand-in prod.
func EC2ErrorAnswer(code string) string { return ec2ErrorAnswer(code) }

// ExecRunner is the production runner, for the assumption tests' git.
var ExecRunner harness.CommandRunner = execCommandRunner{}

// RehearsalAWSProviderTable is the gate's provider "aws" table, for the
// drift test against the provider schema in cli_test.
var RehearsalAWSProviderTable = rehearsalAWSProviderTable
