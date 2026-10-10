package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

// layer3TestGateInputs is what the test path's gate checks a stack
// against. Scaleway needs only the allowlist. AWS needs the region as
// configured (not awsRegion's default), the resolved AMI and its root, and
// the script rendered from the scenario's service. Without a service the
// script stays nil, and the AWS gate refuses a stack it has no script for.
func layer3TestGateInputs(runtime *CommandRuntime, cloud layer3Cloud, sc scenario.Scenario) (awsGateInputs, error) {
	in := allowlistOnlyGateInputs(runtime)
	if cloud != layer3AWS {
		return in, nil
	}
	in.Region = runtime.Config.AWS.Region
	in.AMI = awsResolvedAMI{ID: runtime.AWSLayer3AMI, Root: runtime.AWSLayer3AMIRoot}
	if sc.Service == nil {
		return in, nil
	}
	userData, err := renderAWSUserData(*sc.Service)
	if err != nil {
		return awsGateInputs{}, err
	}
	in.UserData = userData
	return in, nil
}

// appendAWSPostApplyChecks runs after a successful aws apply and before
// any criterion: every resource in the state is placed in the configured
// account, and the instance EC2 launched runs the script infrafactory
// rendered. Either failing means the real probes must not run, so it
// reports false; teardown runs regardless, through destroySandbox's aws
// arm and the scope sweep.
func appendAWSPostApplyChecks(ctx context.Context, runtime *CommandRuntime, sc scenario.Scenario, outputDir string, env map[string]string, stages []StageSummary, failures []FailureSummary) ([]StageSummary, []FailureSummary, bool) {
	checks := []struct {
		stage string
		// run returns the pass stage's detail.
		run func() (string, error)
	}{
		{"account_check", func() (string, error) { return "", awsAccountCheck(runtime, outputDir) }},
		{"user_data_check", func() (string, error) { return awsUserDataCheck(ctx, runtime, sc, outputDir, env) }},
	}
	// The first failure ends the checks: a state that fails placement has
	// no instance worth asking EC2 about.
	for _, check := range checks {
		detail, err := check.run()
		if err != nil {
			stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: check.stage, Status: StageStatusFail})
			failures = append(failures, FailureSummary{
				Layer:   "sandbox_deploy",
				Stage:   check.stage,
				Check:   check.stage,
				Command: "aws post-apply check",
				Detail:  err.Error(),
			})
			return stages, failures, false
		}
		stages = append(stages, StageSummary{Layer: "sandbox_deploy", Stage: check.stage, Status: StageStatusPass, Detail: detail})
	}
	return stages, failures, true
}

func awsAccountCheck(runtime *CommandRuntime, outputDir string) error {
	unplaced, err := harness.UnplacedAWSResources(outputDir, runtime.Config.AWS.AccountID)
	if err != nil {
		return err
	}
	if len(unplaced) > 0 {
		return fmt.Errorf("resources not placed in the configured account: %v", unplaced)
	}
	return nil
}

// awsUserDataCheck returns its pass detail, which names the instance.
func awsUserDataCheck(ctx context.Context, runtime *CommandRuntime, sc scenario.Scenario, outputDir string, env map[string]string) (string, error) {
	if runtime.Deps.AWSEC2 == nil {
		return "", errors.New("no EC2 client to read the instance's user data with")
	}
	if sc.Service == nil {
		return "", errors.New("the scenario has no service: block to render the expected user data from")
	}
	want, err := renderAWSUserData(*sc.Service)
	if err != nil {
		return "", err
	}
	instanceID, err := harness.AWSStateInstanceID(outputDir)
	if err != nil {
		return "", err
	}
	got, err := harness.AWSInstanceUserData(ctx, env, runtime.Deps.AWSEC2, "", instanceID)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(got, want) {
		return "", fmt.Errorf("instance %s does not run the rendered user data", instanceID)
	}
	return fmt.Sprintf("instance %s runs the rendered user data", instanceID), nil
}
