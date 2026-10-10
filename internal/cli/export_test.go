package cli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"

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
