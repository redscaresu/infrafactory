package harness

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// awsReadCallTimeout bounds each identity and preflight call.
const awsReadCallTimeout = 30 * time.Second

// VerifyAWSReadIdentity asks sts:GetCallerIdentity who the read key is,
// through the walled STS client, and refuses unless the answer's account
// is exactly account and, when principal is set, its ARN is exactly
// principal. The account is mandatory: an empty one would match an answer
// missing it, so it refuses with nothing sent. The principal is optional
// (--principal); unlike VerifyAWSIdentity, an empty one skips only the ARN
// comparison.
func VerifyAWSReadIdentity(ctx context.Context, cfg aws.Config, wall *AWSReadWall, account, principal string) error {
	if account == "" {
		return errors.New("aws read identity: the expected account must be set")
	}
	client, err := newAWSReadSTSClient(cfg, wall)
	if err != nil {
		return fmt.Errorf("aws read identity: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, awsReadCallTimeout)
	defer cancel()
	out, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("aws read identity: sts:GetCallerIdentity failed: %w", err)
	}
	if got := aws.ToString(out.Account); got != account {
		return fmt.Errorf("aws read identity: the key belongs to account %q, not --account %q", got, account)
	}
	if got := aws.ToString(out.Arn); principal != "" && got != principal {
		return fmt.Errorf("aws read identity: the key is principal %q, not --principal %q; the match is exact, "+
			"so a key from an assumed role needs its assumed-role ARN, arn:aws:sts::<account>:assumed-role/<role>/<session>, as --principal",
			got, principal)
	}
	return nil
}
