package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// The AWS Layer 3 scope's two SSM parameters (HLD 2026-09-27-aws-web-stack:
// the claim and the stamp). The claim's value is its holder; the stamp's
// is the account id. The stamp is written by hand at setup, and
// infrafactory only ever reads it.
const (
	AWSScopePrefix    = "/infrafactory/layer3/"
	AWSClaimParameter = AWSScopePrefix + "claim"
	AWSStampParameter = AWSScopePrefix + "stamp"
)

// awsClaimTimeout bounds each uncancellable claim step.
const awsClaimTimeout = 20 * time.Second

var (
	// ErrAWSScopeClaimed is a claim held by someone else. Errors wrapping
	// it name the holder.
	ErrAWSScopeClaimed = errors.New("the aws Layer 3 scope is claimed")
	// ErrAWSClaimOutcomeUnknown is a failed put whose read-back failed too:
	// the claim may be ours, so the caller treats it as held.
	ErrAWSClaimOutcomeUnknown = errors.New("the aws Layer 3 claim's outcome is unknown")
	// ErrAWSPreviousClaimDeleted is a take-over whose own take failed
	// after the previous holder's claim was deleted.
	ErrAWSPreviousClaimDeleted = errors.New("the previous holder's aws Layer 3 claim was deleted")
)

// AWSScopeClaimedError is ErrAWSScopeClaimed naming the holder, for a
// caller that names it in a command.
type AWSScopeClaimedError struct{ Holder string }

func (e *AWSScopeClaimedError) Error() string {
	return fmt.Sprintf("%v by %q", ErrAWSScopeClaimed, e.Holder)
}

func (e *AWSScopeClaimedError) Unwrap() error { return ErrAWSScopeClaimed }

// awsClaimHolderRe is <run id>@<host>:<pid>. A holder goes into a command
// the operator pastes, so nothing else is accepted.
var awsClaimHolderRe = regexp.MustCompile(`^[A-Za-z0-9._+-]+@[A-Za-z0-9.-]+:[0-9]+$`)

// NewAWSClaimHolder returns runID@hostname:pid, unique to this process.
func NewAWSClaimHolder(runID string) (string, error) {
	host, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("aws scope claim: reading the hostname: %w", err)
	}
	holder := fmt.Sprintf("%s@%s:%d", runID, host, os.Getpid())
	if err := checkAWSClaimHolder(holder); err != nil {
		return "", err
	}
	return holder, nil
}

func checkAWSClaimHolder(holder string) error {
	if !awsClaimHolderRe.MatchString(holder) {
		return fmt.Errorf("aws scope claim: %q is not a holder of the form <run id>@<host>:<pid>", holder)
	}
	return nil
}

// TakeAWSClaim claims the scope for holder with a PutParameter that never
// overwrites. When the put fails, whatever the error, the stored value
// decides: holder means a retried put reported our own landed attempt as
// a conflict, so the claim is taken; another value is ErrAWSScopeClaimed
// naming it; a failed read is ErrAWSClaimOutcomeUnknown.
func TakeAWSClaim(ctx context.Context, env map[string]string, doer ssm.HTTPClient, endpoint, holder string) error {
	if err := checkAWSClaimHolder(holder); err != nil {
		return err
	}
	client, err := newAWSSSMClient(env, doer, endpoint)
	if err != nil {
		return err
	}
	return takeAWSClaim(ctx, client, holder)
}

// takeAWSClaim is not cancellable by the caller, as Scaleway's project
// create is not (run_project_lifecycle.go): an interrupt inside the put
// could leave a claim taken that the run never learns it holds.
func takeAWSClaim(ctx context.Context, client *ssm.Client, holder string) error {
	ctx = context.WithoutCancel(ctx)
	putCtx, cancelPut := context.WithTimeout(ctx, awsClaimTimeout)
	defer cancelPut()
	_, putErr := client.PutParameter(putCtx, &ssm.PutParameterInput{
		Name:      aws.String(AWSClaimParameter),
		Value:     aws.String(holder),
		Type:      ssmtypes.ParameterTypeString,
		Overwrite: aws.Bool(false),
	})
	if putErr == nil {
		return nil
	}

	getCtx, cancelGet := context.WithTimeout(ctx, awsClaimTimeout)
	defer cancelGet()
	value, found, err := getAWSParameter(getCtx, client, AWSClaimParameter)
	switch {
	case err != nil:
		return fmt.Errorf("%w: ssm:PutParameter %s failed (%v), and reading it back failed: %w", ErrAWSClaimOutcomeUnknown, AWSClaimParameter, putErr, err)
	case found && value == holder:
		return nil
	case found:
		return &AWSScopeClaimedError{Holder: value}
	}
	return fmt.Errorf("aws scope claim: ssm:PutParameter %s failed and no claim is stored: %w", AWSClaimParameter, putErr)
}

// ReadAWSClaimHolder returns the claim's holder, with held false when
// there is no claim. A stored value that is not a holder is an error, so
// nothing else reaches a command the operator pastes.
func ReadAWSClaimHolder(ctx context.Context, env map[string]string, doer ssm.HTTPClient, endpoint string) (holder string, held bool, err error) {
	client, err := newAWSSSMClient(env, doer, endpoint)
	if err != nil {
		return "", false, err
	}
	holder, held, err = getAWSParameter(ctx, client, AWSClaimParameter)
	if err != nil {
		return "", false, fmt.Errorf("aws scope claim: %w", err)
	}
	if held {
		if err := checkAWSClaimHolder(holder); err != nil {
			return "", false, err
		}
	}
	return holder, held, nil
}

// ReleaseAWSClaim deletes the claim only if holder holds it.
func ReleaseAWSClaim(ctx context.Context, env map[string]string, doer ssm.HTTPClient, endpoint, holder string) error {
	if err := checkAWSClaimHolder(holder); err != nil {
		return err
	}
	client, err := newAWSSSMClient(env, doer, endpoint)
	if err != nil {
		return err
	}
	return deleteAWSClaimHeldBy(ctx, client, holder)
}

// TakeOverAWSClaim moves the claim from previous to holder, for reap. It
// refuses a previous whose process is still running on this host, and a
// claim whose value is not previous. It then deletes the claim and takes
// it as TakeAWSClaim does, so a holder that took the scope in between is
// refused by name, never overwritten.
func TakeOverAWSClaim(ctx context.Context, env map[string]string, doer ssm.HTTPClient, endpoint, previous, holder string) error {
	for _, h := range []string{previous, holder} {
		if err := checkAWSClaimHolder(h); err != nil {
			return err
		}
	}
	if err := refuseLiveLocalHolder(previous); err != nil {
		return err
	}
	client, err := newAWSSSMClient(env, doer, endpoint)
	if err != nil {
		return err
	}
	// An interrupt must not fall between the delete and the take, so
	// neither is cancellable.
	deleteCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), awsClaimTimeout)
	defer cancel()
	if err := deleteAWSClaimHeldBy(deleteCtx, client, previous); err != nil {
		return err
	}
	if err := takeAWSClaim(ctx, client, holder); err != nil {
		return fmt.Errorf("%w: %w", ErrAWSPreviousClaimDeleted, err)
	}
	return nil
}

// deleteAWSClaimHeldBy deletes the claim when its value is want, and
// deletes nothing when it is absent or anything else.
func deleteAWSClaimHeldBy(ctx context.Context, client *ssm.Client, want string) error {
	value, found, err := getAWSParameter(ctx, client, AWSClaimParameter)
	switch {
	case err != nil:
		return fmt.Errorf("aws scope claim: %w", err)
	case !found:
		return fmt.Errorf("aws scope claim: refusing to delete %s: no claim is held, and %q expected to hold it", AWSClaimParameter, want)
	case value != want:
		return fmt.Errorf("aws scope claim: refusing to delete %s: %w, not %q", AWSClaimParameter, &AWSScopeClaimedError{Holder: value}, want)
	}
	// ponytail: DeleteParameter takes only a Name, so a claim retaken
	// between the Get above and this Delete would be deleted. SSM has no
	// conditional delete; serial runs and an operator-driven takeover are
	// the ceiling. Revisit if two runs can ever race for one scope.
	_, err = client.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String(AWSClaimParameter)})
	// ParameterNotFound is a retried delete reporting our own landed
	// attempt: either way the claim is gone, which is what was asked.
	var notFound *ssmtypes.ParameterNotFound
	if err != nil && !errors.As(err, &notFound) {
		return fmt.Errorf("aws scope claim: ssm:DeleteParameter %s failed: %w", AWSClaimParameter, err)
	}
	return nil
}

// refuseLiveLocalHolder refuses a holder (already validated) whose
// process is running on this host: that run still owns the scope.
func refuseLiveLocalHolder(holder string) error {
	_, hostPID, _ := strings.Cut(holder, "@")
	host, pidText, _ := strings.Cut(hostPID, ":")
	self, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("aws scope claim: reading the hostname: %w", err)
	}
	if host != self {
		return nil
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		return fmt.Errorf("aws scope claim: holder %q has pid %q: %w", holder, pidText, err)
	}
	// Signal 0 checks for the process without signalling it; EPERM means
	// it exists under another user.
	if err := syscall.Kill(pid, 0); err == nil || errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("aws scope claim: refusing to take over from %q: process %d on this host is still running", holder, pid)
	}
	return nil
}

// AssertAWSScopeStamp refuses unless the stamp holds account. It only
// reads.
func AssertAWSScopeStamp(ctx context.Context, env map[string]string, doer ssm.HTTPClient, endpoint, account string) error {
	// An empty expectation would match nothing and prove nothing.
	if account == "" {
		return errors.New("aws scope stamp: the expected account must be set")
	}
	client, err := newAWSSSMClient(env, doer, endpoint)
	if err != nil {
		return err
	}
	value, found, err := getAWSParameter(ctx, client, AWSStampParameter)
	switch {
	case err != nil:
		return fmt.Errorf("aws scope stamp: %w", err)
	case !found:
		return fmt.Errorf("aws scope stamp: %s does not exist, so this account is not a stamped Layer 3 scope; it is written at setup (docs/operations.md § Layer 3 (AWS))", AWSStampParameter)
	case value != account:
		return fmt.Errorf("aws scope stamp: %s holds %q, not the configured account %q", AWSStampParameter, value, account)
	}
	return nil
}

// AssertNoAWSDefaultVPC refuses if the region has a default VPC, which
// setup deletes. It reads every page and checks IsDefault itself, so a
// filter the server ignored could not pass it.
func AssertNoAWSDefaultVPC(ctx context.Context, env map[string]string, doer ec2.HTTPClient, endpoint string) error {
	client, err := newAWSEC2Client(env, doer, endpoint)
	if err != nil {
		return err
	}
	pages := ec2.NewDescribeVpcsPaginator(client, &ec2.DescribeVpcsInput{})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("aws default vpc check: DescribeVpcs failed: %w", err)
		}
		for _, vpc := range page.Vpcs {
			if aws.ToBool(vpc.IsDefault) {
				return fmt.Errorf("aws default vpc check: %s is the region's default VPC, which setup deletes (docs/operations.md § Layer 3 (AWS))", aws.ToString(vpc.VpcId))
			}
		}
	}
	return nil
}

// getAWSParameter returns name's value, with found false when SSM
// answers ParameterNotFound.
func getAWSParameter(ctx context.Context, client *ssm.Client, name string) (value string, found bool, err error) {
	out, err := client.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(name)})
	var notFound *ssmtypes.ParameterNotFound
	if errors.As(err, &notFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("ssm:GetParameter %s failed: %w", name, err)
	}
	if out.Parameter == nil || out.Parameter.Value == nil {
		return "", false, fmt.Errorf("ssm:GetParameter %s returned no value", name)
	}
	return *out.Parameter.Value, true, nil
}
