package harness

import (
	"context"
	"errors"
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

// newAWSSSMClient builds an SSM client as NewAWSSTSClient builds its STS
// one: the sealed env's static key and region, never the SDK default
// chain. endpoint "" means real SSM in that region. doer is required, so
// no caller reaches SSM through a client it did not choose.
func newAWSSSMClient(env map[string]string, doer ssm.HTTPClient, endpoint string) (*ssm.Client, error) {
	if doer == nil {
		return nil, errors.New("aws ssm client: needs an HTTP client, and none was given")
	}
	cfg, err := sealedAWSConfig(env, "aws ssm client")
	if err != nil {
		return nil, err
	}
	if endpoint == "" {
		endpoint = "https://ssm." + cfg.Region + ".amazonaws.com"
	}
	return ssm.NewFromConfig(cfg, func(o *ssm.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.HTTPClient = doer
	}), nil
}

// ResolveAWSAMIFromSSM reads AWSAL2023AMIParameter through newAWSSSMClient.
func ResolveAWSAMIFromSSM(ctx context.Context, env map[string]string, doer ssm.HTTPClient, endpoint string) (string, error) {
	client, err := newAWSSSMClient(env, doer, endpoint)
	if err != nil {
		return "", fmt.Errorf("aws ami: reading %s: %w", AWSAL2023AMIParameter, err)
	}

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
