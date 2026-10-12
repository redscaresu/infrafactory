package harness

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

// awsDryRunRefusedCode is the one answer the preflight accepts: EC2
// refused the action for the placeholder resource. With DryRun=true, EC2
// answers DryRunOperation when the key's grant reaches that resource.
const awsDryRunRefusedCode = "UnauthorizedOperation"

// PreflightAWSReadKey probes the reap's EC2 actions, the DryRun table. It
// sends each once with DryRun=true and a placeholder id through the
// walled EC2 client, and passes only when every answer is the API error
// UnauthorizedOperation; a success, any other code (DryRunOperation
// among them) or no API answer at all refuses, naming the action.
//
// A pass proves only that the key holds no grant for those actions that
// reaches a resource it has never seen. It proves nothing about actions
// outside the table: a key that can RunInstances, CreateVpc or
// AuthorizeSecurityGroupIngress passes. Nor about a grant scoped to real
// ARNs or tags, which the placeholder resource matches neither of. What
// stops every non-reader request, on every service, is the wall, in
// process at runtime; DryRun is EC2-only.
func PreflightAWSReadKey(ctx context.Context, cfg aws.Config, wall *AWSReadWall) error {
	return preflightAWSRead(ctx, cfg, wall, AWSReaderDryRunActions)
}

func preflightAWSRead(ctx context.Context, cfg aws.Config, wall *AWSReadWall, actions []string) error {
	// An empty table would pass having proved nothing.
	if len(actions) == 0 {
		return errors.New("aws read preflight: refusing: the DryRun table is empty")
	}
	// A row with no placeholder request is never skipped: nothing is sent.
	var missing []string
	for _, action := range actions {
		if awsDryRunPlaceholders[action] == nil {
			missing = append(missing, action)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("aws read preflight: refusing: DryRun-table rows with no placeholder request: %s", strings.Join(missing, ", "))
	}
	client, err := newAWSReadEC2Client(cfg, wall)
	if err != nil {
		return fmt.Errorf("aws read preflight: %w", err)
	}
	var errs []error
	for _, action := range actions {
		if err := awsDryRunRefused(ctx, client, action); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("aws read preflight: refusing: each DryRun-table action must answer %s: %w", awsDryRunRefusedCode, errors.Join(errs...))
	}
	return nil
}

// awsDryRunRefused sends action's placeholder request and returns nil
// only when EC2 answered UnauthorizedOperation.
func awsDryRunRefused(ctx context.Context, client *ec2.Client, action string) error {
	ctx, cancel := context.WithTimeout(ctx, awsReadCallTimeout)
	defer cancel()
	err := awsDryRunPlaceholders[action](ctx, client)
	var apiErr smithy.APIError
	switch {
	case err == nil:
		return fmt.Errorf("%s succeeded with DryRun=true", action)
	case !errors.As(err, &apiErr):
		return fmt.Errorf("%s got no EC2 answer: %w", action, err)
	case apiErr.ErrorCode() != awsDryRunRefusedCode:
		return fmt.Errorf("%s answered %s: %w", action, apiErr.ErrorCode(), err)
	}
	return nil
}

type awsDryRunSend func(context.Context, *ec2.Client) error

// awsDryRun sends in through call, an *ec2.Client method.
func awsDryRun[In, Out any](call func(*ec2.Client, context.Context, In, ...func(*ec2.Options)) (Out, error), in In) awsDryRunSend {
	return func(ctx context.Context, client *ec2.Client) error {
		_, err := call(client, ctx, in)
		return err
	}
}

// awsPlaceholderID is a syntactically valid EC2 id: prefix, a dash and 17
// hex digits. It names no real resource, and DryRun touches none.
func awsPlaceholderID(prefix string) string {
	return prefix + "-0123456789abcdef0"
}

// awsPlaceholderRule is one rule for the revokes, which name a group and
// what to revoke from it. 192.0.2.0/24 is TEST-NET-1 (RFC 5737).
var awsPlaceholderRule = []types.IpPermission{{
	IpProtocol: aws.String("tcp"), FromPort: aws.Int32(443), ToPort: aws.Int32(443),
	IpRanges: []types.IpRange{{CidrIp: aws.String("192.0.2.0/24")}},
}}

// awsDryRunPlaceholders holds each DryRun-table action's request: DryRun
// and the action's identifying parameter, with a placeholder id. No input
// here has an idempotency token, so the SDK never writes to one.
var awsDryRunPlaceholders = map[string]awsDryRunSend{
	"TerminateInstances": awsDryRun((*ec2.Client).TerminateInstances, &ec2.TerminateInstancesInput{
		DryRun: aws.Bool(true), InstanceIds: []string{awsPlaceholderID("i")}}),
	"DeleteNatGateway": awsDryRun((*ec2.Client).DeleteNatGateway, &ec2.DeleteNatGatewayInput{
		DryRun: aws.Bool(true), NatGatewayId: aws.String(awsPlaceholderID("nat"))}),
	"DisassociateAddress": awsDryRun((*ec2.Client).DisassociateAddress, &ec2.DisassociateAddressInput{
		DryRun: aws.Bool(true), AssociationId: aws.String(awsPlaceholderID("eipassoc"))}),
	"ReleaseAddress": awsDryRun((*ec2.Client).ReleaseAddress, &ec2.ReleaseAddressInput{
		DryRun: aws.Bool(true), AllocationId: aws.String(awsPlaceholderID("eipalloc"))}),
	"DeleteLaunchTemplate": awsDryRun((*ec2.Client).DeleteLaunchTemplate, &ec2.DeleteLaunchTemplateInput{
		DryRun: aws.Bool(true), LaunchTemplateId: aws.String(awsPlaceholderID("lt"))}),
	"DeregisterImage": awsDryRun((*ec2.Client).DeregisterImage, &ec2.DeregisterImageInput{
		DryRun: aws.Bool(true), ImageId: aws.String(awsPlaceholderID("ami"))}),
	"DeleteSnapshot": awsDryRun((*ec2.Client).DeleteSnapshot, &ec2.DeleteSnapshotInput{
		DryRun: aws.Bool(true), SnapshotId: aws.String(awsPlaceholderID("snap"))}),
	"DeleteVolume": awsDryRun((*ec2.Client).DeleteVolume, &ec2.DeleteVolumeInput{
		DryRun: aws.Bool(true), VolumeId: aws.String(awsPlaceholderID("vol"))}),
	"DetachNetworkInterface": awsDryRun((*ec2.Client).DetachNetworkInterface, &ec2.DetachNetworkInterfaceInput{
		DryRun: aws.Bool(true), AttachmentId: aws.String(awsPlaceholderID("eni-attach"))}),
	"DeleteNetworkInterface": awsDryRun((*ec2.Client).DeleteNetworkInterface, &ec2.DeleteNetworkInterfaceInput{
		DryRun: aws.Bool(true), NetworkInterfaceId: aws.String(awsPlaceholderID("eni"))}),
	"DeleteKeyPair": awsDryRun((*ec2.Client).DeleteKeyPair, &ec2.DeleteKeyPairInput{
		DryRun: aws.Bool(true), KeyPairId: aws.String(awsPlaceholderID("key"))}),
	"RevokeSecurityGroupIngress": awsDryRun((*ec2.Client).RevokeSecurityGroupIngress, &ec2.RevokeSecurityGroupIngressInput{
		DryRun: aws.Bool(true), GroupId: aws.String(awsPlaceholderID("sg")), IpPermissions: awsPlaceholderRule}),
	"RevokeSecurityGroupEgress": awsDryRun((*ec2.Client).RevokeSecurityGroupEgress, &ec2.RevokeSecurityGroupEgressInput{
		DryRun: aws.Bool(true), GroupId: aws.String(awsPlaceholderID("sg")), IpPermissions: awsPlaceholderRule}),
	"DeleteSecurityGroup": awsDryRun((*ec2.Client).DeleteSecurityGroup, &ec2.DeleteSecurityGroupInput{
		DryRun: aws.Bool(true), GroupId: aws.String(awsPlaceholderID("sg"))}),
	"DisassociateRouteTable": awsDryRun((*ec2.Client).DisassociateRouteTable, &ec2.DisassociateRouteTableInput{
		DryRun: aws.Bool(true), AssociationId: aws.String(awsPlaceholderID("rtbassoc"))}),
	"DeleteRouteTable": awsDryRun((*ec2.Client).DeleteRouteTable, &ec2.DeleteRouteTableInput{
		DryRun: aws.Bool(true), RouteTableId: aws.String(awsPlaceholderID("rtb"))}),
	"DeleteSubnet": awsDryRun((*ec2.Client).DeleteSubnet, &ec2.DeleteSubnetInput{
		DryRun: aws.Bool(true), SubnetId: aws.String(awsPlaceholderID("subnet"))}),
	"DetachInternetGateway": awsDryRun((*ec2.Client).DetachInternetGateway, &ec2.DetachInternetGatewayInput{
		DryRun: aws.Bool(true), InternetGatewayId: aws.String(awsPlaceholderID("igw")), VpcId: aws.String(awsPlaceholderID("vpc"))}),
	"DeleteInternetGateway": awsDryRun((*ec2.Client).DeleteInternetGateway, &ec2.DeleteInternetGatewayInput{
		DryRun: aws.Bool(true), InternetGatewayId: aws.String(awsPlaceholderID("igw"))}),
	"DeleteVpc": awsDryRun((*ec2.Client).DeleteVpc, &ec2.DeleteVpcInput{
		DryRun: aws.Bool(true), VpcId: aws.String(awsPlaceholderID("vpc"))}),
}
