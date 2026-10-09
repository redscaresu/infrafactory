package cli

import (
	"context"
	"fmt"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// resolveAWSLayer3AMI is the aws Layer 3 AMI resolve (ADR-0039 decision
// 7): STS, then the AL2023 id from SSM, then that image's root from EC2.
// generate, run and test call it once, before generation or the gate and
// outside the interrupt guard, since it takes no claim and leaves nothing
// to reap (ADR-0040 decisions 5-6). It is the only code that sets
// AWSLayer3AMI and AWSLayer3AMIRoot. Any other cloud, or Layer 3 off,
// makes no call and gets no stage.
func resolveAWSLayer3AMI(ctx context.Context, runtime *CommandRuntime, cloud layer3Cloud) ([]StageSummary, error) {
	if cloud != layer3AWS || !runtime.Config.Validation.Layers.SandboxDeploy.Enabled {
		return nil, nil
	}
	id, root, err := awsLayer3AMI(ctx, runtime)
	if err != nil {
		return []StageSummary{{Layer: "sandbox_deploy", Stage: StageAWSAMIResolve, Status: StageStatusFail}},
			fmt.Errorf("%s: %w", StageAWSAMIResolve, err)
	}
	runtime.AWSLayer3AMI, runtime.AWSLayer3AMIRoot = id, root
	return []StageSummary{{
		Layer: "sandbox_deploy", Stage: StageAWSAMIResolve, Status: StageStatusPass,
		Detail: fmt.Sprintf("%s names %s, whose root is %d GiB %s, deleted on termination: %t",
			harness.AWSAL2023AMIParameter, id, root.SizeGiB, root.VolumeType, root.DeleteOnTermination),
	}}, nil
}

func awsLayer3AMI(ctx context.Context, runtime *CommandRuntime) (string, harness.AWSAMIRoot, error) {
	env, err := awsVerifiedEnv(ctx, runtime)
	if err != nil {
		return "", harness.AWSAMIRoot{}, err
	}
	id, err := harness.ResolveAWSAMIFromSSM(ctx, env, runtime.Deps.AWSSSM, "")
	if err != nil {
		return "", harness.AWSAMIRoot{}, err
	}
	root, err := harness.DescribeAWSAMIRoot(ctx, env, runtime.Deps.AWSEC2, "", id)
	if err != nil {
		return "", harness.AWSAMIRoot{}, err
	}
	return id, root, nil
}
