package harness

import (
	"context"
	"fmt"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// AWSLayer2AMI is the AMI id the model writes at Layer 2: fakeaws's answer
// for AWSAL2023AMIParameter (ADR-0039 decision 7).
// TestAWSAMIParityAgainstFakeaws holds the two together.
const AWSLayer2AMI = "ami-0al2023x8664"

// AWSAL2023AMIParameter is the AWS public SSM parameter naming the latest
// Amazon Linux 2023 x86_64 AMI in the reader's region.
const AWSAL2023AMIParameter = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"

// awsAMIIDRe admits fakeaws's id as well as real AWS's hex ones. The id
// is written into the prompt verbatim, so nothing else gets through.
var awsAMIIDRe = regexp.MustCompile(`^ami-[0-9a-z]+$`)

// ResolveAWSAMIFromSSM reads AWSAL2023AMIParameter through an SSM client
// built as NewAWSSTSClient builds its STS one: the sealed env's static key
// and region, never the SDK default chain. endpoint "" means real SSM in
// that region. doer is required, so no caller reaches SSM through a client
// it did not choose.
func ResolveAWSAMIFromSSM(ctx context.Context, env map[string]string, doer ssm.HTTPClient, endpoint string) (string, error) {
	if doer == nil {
		return "", fmt.Errorf("aws ami: reading %s needs an HTTP client, and none was given", AWSAL2023AMIParameter)
	}
	cfg, err := sealedAWSConfig(env, "aws ssm client")
	if err != nil {
		return "", err
	}
	if endpoint == "" {
		endpoint = "https://ssm." + cfg.Region + ".amazonaws.com"
	}
	client := ssm.NewFromConfig(cfg, func(o *ssm.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.HTTPClient = doer
	})

	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(AWSAL2023AMIParameter)})
	if err != nil {
		return "", fmt.Errorf("aws ami: ssm:GetParameter %s failed: %w", AWSAL2023AMIParameter, err)
	}
	var id string
	if out.Parameter != nil {
		id = aws.ToString(out.Parameter.Value)
	}
	if !awsAMIIDRe.MatchString(id) {
		return "", fmt.Errorf("aws ami: %s holds %q, which is not an AMI id", AWSAL2023AMIParameter, id)
	}
	return id, nil
}
