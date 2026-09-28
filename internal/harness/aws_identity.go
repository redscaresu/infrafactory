package harness

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// VerifyAWSIdentity asks sts:GetCallerIdentity who the sealed env's key
// is, through NewAWSSTSClient, and refuses unless the answer is exactly
// wantAccount and wantPrincipal. A key for the wrong account, or for a
// broader principal in the right one, must stop the run before anything
// is applied (ADR-0023 rule 2).
func VerifyAWSIdentity(ctx context.Context, env map[string]string, doer sts.HTTPClient, endpoint, wantAccount, wantPrincipal string) error {
	// An empty expectation would match a response missing that field.
	if wantAccount == "" || wantPrincipal == "" {
		return errors.New("aws identity check: the expected account and principal must both be set")
	}
	client, err := NewAWSSTSClient(env, doer, endpoint)
	if err != nil {
		return err
	}
	out, err := client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("aws identity check: sts:GetCallerIdentity failed: %w", err)
	}
	if got := aws.ToString(out.Account); got != wantAccount {
		return fmt.Errorf("aws identity check: the key belongs to account %q, not the configured account %q", got, wantAccount)
	}
	if got := aws.ToString(out.Arn); got != wantPrincipal {
		return fmt.Errorf("aws identity check: the key is principal %q, not the configured principal %q", got, wantPrincipal)
	}
	return nil
}
