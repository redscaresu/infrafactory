package harness

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

// ReapAWSScope deletes strays, a sweep's findings, from the AWS Layer 3
// scope: ADR-0023 rule 4 for AWS. Before anything reaches EC2 it proves
// the key is principal in account, the stamp holds account, and holder
// holds the claim, so no caller reaches a delete without that gate. It
// then runs AWSReapSteps in order. A failed item does not stop the rest:
// the error joins one per failed item, and the caller's next sweep is the
// verdict. It never touches the claim and deletes no SSM parameter: the
// key may write only the claim. sleep nil means a real sleep.
func ReapAWSScope(ctx context.Context, env map[string]string, doers AWSDoers, endpoints AWSEndpoints, sleep func(context.Context, time.Duration) error, account, principal, holder string, strays []AWSStray) error {
	if err := VerifyAWSIdentity(ctx, env, doers.STS, endpoints.STS, account, principal); err != nil {
		return fmt.Errorf("aws scope reap: %w", err)
	}
	if err := AssertAWSScopeStamp(ctx, env, doers.SSM, endpoints.SSM, account); err != nil {
		return fmt.Errorf("aws scope reap: %w", err)
	}
	got, held, err := ReadAWSClaimHolder(ctx, env, doers.SSM, endpoints.SSM)
	switch {
	case err != nil:
		return fmt.Errorf("aws scope reap: %w", err)
	case !held:
		return fmt.Errorf("aws scope reap: refusing: no claim is held, and %q expected to hold it", holder)
	case got != holder:
		return fmt.Errorf("aws scope reap: refusing: %w by %q, not %q", ErrAWSScopeClaimed, got, holder)
	}
	client, err := newAWSEC2Client(env, doers.EC2, endpoints.EC2)
	if err != nil {
		return fmt.Errorf("aws scope reap: %w", err)
	}
	if sleep == nil {
		sleep = sleepContext
	}
	r := awsReaper{ec2: client, sleep: sleep}

	ids := map[string][]string{}
	for _, stray := range strays {
		ids[stray.Collection] = append(ids[stray.Collection], stray.ID)
	}
	var errs []error
	for _, step := range AWSReapSteps {
		for _, err := range step.reap(ctx, r, ids[step.Collection]) {
			errs = append(errs, fmt.Errorf("aws scope reap: %s %w", step.Collection, err))
		}
		delete(ids, step.Collection)
	}
	for _, stray := range strays {
		if _, left := ids[stray.Collection]; left {
			errs = append(errs, fmt.Errorf("aws scope reap: %s: reap does not delete this; remove it by hand", stray))
		}
	}
	return errors.Join(errs...)
}

// AWSReapStep deletes one swept collection's strays. EC2Actions is every
// EC2 action it sends, for the IAM policy test.
type AWSReapStep struct {
	Collection string
	EC2Actions []string
	reap       func(context.Context, awsReaper, []string) []error
}

// AWSReapSteps run in real AWS dependency order. Every swept collection
// but the SSM parameters has one.
var AWSReapSteps = []AWSReapStep{
	// An instance holds volumes, network interfaces, groups and public
	// addresses in a subnet.
	{"instances", []string{"TerminateInstances", "DescribeInstances"}, twoPhase(terminateAWSInstance, awaitAWSInstanceTerminated)},
	// A NAT gateway holds an Elastic IP and a network interface in a subnet.
	{"nat gateways", []string{"DeleteNatGateway", "DescribeNatGateways"}, twoPhase(deleteAWSNATGateway, awaitAWSNATGatewayDeleted)},
	// DetachInternetGateway fails while the VPC has mapped public addresses.
	{"elastic ips", []string{"DescribeAddresses", "DisassociateAddress", "ReleaseAddress"}, eachAWS(releaseAWSAddress)},
	{"launch templates", []string{"DeleteLaunchTemplate"}, eachAWS(deleteAWSLaunchTemplate)},
	// A registered image holds its snapshots.
	{"images", []string{"DeregisterImage"}, eachAWS(deregisterAWSImage)},
	{"snapshots", []string{"DeleteSnapshot"}, eachAWS(deleteAWSSnapshot)},
	{"volumes", []string{"DescribeVolumes", "DeleteVolume"}, eachAWS(deleteAWSVolume)},
	// A network interface holds its groups and its subnet.
	{"network interfaces", []string{"DescribeNetworkInterfaces", "DetachNetworkInterface", "DeleteNetworkInterface"}, eachAWS(deleteAWSNetworkInterface)},
	{"key pairs", []string{"DeleteKeyPair"}, eachAWS(deleteAWSKeyPair)},
	// A group another group's rule names cannot be deleted, so every
	// group's rules go before any group.
	{"security groups", []string{"DescribeSecurityGroups", "RevokeSecurityGroupIngress", "RevokeSecurityGroupEgress", "DeleteSecurityGroup"}, twoPhase(revokeAWSGroupRules, deleteAWSSecurityGroup)},
	// Before the subnets: deleting a subnet drops its association
	// unannounced, and fakeaws cascades it the same way, so disassociating
	// first is the order that sends every DisassociateRouteTable.
	{"route tables", []string{"DescribeRouteTables", "DisassociateRouteTable", "DeleteRouteTable"}, eachAWS(deleteAWSRouteTable)},
	{"subnets", []string{"DeleteSubnet"}, eachAWS(deleteAWSSubnet)},
	{"internet gateways", []string{"DescribeInternetGateways", "DetachInternetGateway", "DeleteInternetGateway"}, eachAWS(deleteAWSInternetGateway)},
	// A VPC cannot be deleted while anything above is in it.
	{"vpcs", []string{"DeleteVpc"}, eachAWS(deleteAWSVPC)},
}

type awsReaper struct {
	ec2   *ec2.Client
	sleep func(context.Context, time.Duration) error
}

// settle polls ready until it reports true, sleeping between polls, and
// fails after AWSSweepMaxPolls.
func (r awsReaper) settle(ctx context.Context, id, want string, ready func() (bool, error)) error {
	for poll := 1; ; poll++ {
		done, err := ready()
		if err != nil {
			return fmt.Errorf("%s: waiting until %s: %w", id, want, err)
		}
		if done {
			return nil
		}
		if poll == AWSSweepMaxPolls {
			return fmt.Errorf("%s: not %s after %d polls", id, want, poll)
		}
		if err := r.sleep(ctx, awsSweepSettleInterval); err != nil {
			return fmt.Errorf("%s: waiting until %s: %w", id, want, err)
		}
	}
}

type awsReapItem func(context.Context, awsReaper, string) error

// eachAWS reaps each id in turn, with one error per failed id.
func eachAWS(reap awsReapItem) func(context.Context, awsReaper, []string) []error {
	return func(ctx context.Context, r awsReaper, ids []string) []error {
		var errs []error
		for _, id := range ids {
			if err := reap(ctx, r, id); err != nil {
				errs = append(errs, err)
			}
		}
		return errs
	}
}

// twoPhase runs first on every id, then second on each id first did not
// fail on.
func twoPhase(first, second awsReapItem) func(context.Context, awsReaper, []string) []error {
	return func(ctx context.Context, r awsReaper, ids []string) []error {
		var errs []error
		var passed []string
		for _, id := range ids {
			if err := first(ctx, r, id); err != nil {
				errs = append(errs, err)
				continue
			}
			passed = append(passed, id)
		}
		return append(errs, eachAWS(second)(ctx, r, passed)...)
	}
}

// awsReapErr names id and action on a failed call. A NotFound is not a
// failure: the item is already gone.
func awsReapErr(id, action string, err error) error {
	if err == nil || awsNotFound(err) {
		return nil
	}
	return fmt.Errorf("%s: ec2:%s failed: %w", id, action, err)
}

func awsNotFound(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && strings.HasSuffix(apiErr.ErrorCode(), "NotFound")
}

// awsOne is the item named id from a Describe by id, or nil when it is
// gone: the Describe answered NotFound or left id out. id is matched
// here, so a server that ignored the id cannot hand back another item.
func awsOne[T any](id string, idOf func(T) *string, describe func() ([]T, error)) (*T, error) {
	items, err := describe()
	if awsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for i := range items {
		if aws.ToString(idOf(items[i])) == id {
			return &items[i], nil
		}
	}
	return nil, nil
}

func terminateAWSInstance(ctx context.Context, r awsReaper, id string) error {
	_, err := r.ec2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{id}})
	return awsReapErr(id, "TerminateInstances", err)
}

func awaitAWSInstanceTerminated(ctx context.Context, r awsReaper, id string) error {
	return r.settle(ctx, id, "terminated", func() (bool, error) {
		instance, err := awsOne(id, func(i types.Instance) *string { return i.InstanceId }, func() ([]types.Instance, error) {
			out, err := r.ec2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{id}})
			if err != nil {
				return nil, err
			}
			var instances []types.Instance
			for _, reservation := range out.Reservations {
				instances = append(instances, reservation.Instances...)
			}
			return instances, nil
		})
		return instance == nil || instance.State != nil && instance.State.Name == types.InstanceStateNameTerminated, err
	})
}

func deleteAWSNATGateway(ctx context.Context, r awsReaper, id string) error {
	_, err := r.ec2.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{NatGatewayId: aws.String(id)})
	return awsReapErr(id, "DeleteNatGateway", err)
}

func awaitAWSNATGatewayDeleted(ctx context.Context, r awsReaper, id string) error {
	return r.settle(ctx, id, "deleted", func() (bool, error) {
		gateway, err := awsOne(id, func(g types.NatGateway) *string { return g.NatGatewayId }, func() ([]types.NatGateway, error) {
			out, err := r.ec2.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{NatGatewayIds: []string{id}})
			if err != nil {
				return nil, err
			}
			return out.NatGateways, nil
		})
		return gateway == nil || gateway.State == types.NatGatewayStateDeleted, err
	})
}

func releaseAWSAddress(ctx context.Context, r awsReaper, id string) error {
	address, err := awsOne(id, func(a types.Address) *string { return a.AllocationId }, func() ([]types.Address, error) {
		out, err := r.ec2.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{AllocationIds: []string{id}})
		if err != nil {
			return nil, err
		}
		return out.Addresses, nil
	})
	if err != nil || address == nil {
		return awsReapErr(id, "DescribeAddresses", err)
	}
	if address.AssociationId != nil {
		_, err := r.ec2.DisassociateAddress(ctx, &ec2.DisassociateAddressInput{AssociationId: address.AssociationId})
		if err := awsReapErr(id, "DisassociateAddress", err); err != nil {
			return err
		}
	}
	_, err = r.ec2.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(id)})
	return awsReapErr(id, "ReleaseAddress", err)
}

func deleteAWSLaunchTemplate(ctx context.Context, r awsReaper, id string) error {
	_, err := r.ec2.DeleteLaunchTemplate(ctx, &ec2.DeleteLaunchTemplateInput{LaunchTemplateId: aws.String(id)})
	return awsReapErr(id, "DeleteLaunchTemplate", err)
}

func deregisterAWSImage(ctx context.Context, r awsReaper, id string) error {
	_, err := r.ec2.DeregisterImage(ctx, &ec2.DeregisterImageInput{ImageId: aws.String(id)})
	return awsReapErr(id, "DeregisterImage", err)
}

func deleteAWSSnapshot(ctx context.Context, r awsReaper, id string) error {
	_, err := r.ec2.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: aws.String(id)})
	return awsReapErr(id, "DeleteSnapshot", err)
}

// A volume can be deleted once available, when the instances above that
// held it are terminated.
func deleteAWSVolume(ctx context.Context, r awsReaper, id string) error {
	var volume *types.Volume
	err := r.settle(ctx, id, "available", func() (bool, error) {
		var err error
		volume, err = awsOne(id, func(v types.Volume) *string { return v.VolumeId }, func() ([]types.Volume, error) {
			out, err := r.ec2.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{VolumeIds: []string{id}})
			if err != nil {
				return nil, err
			}
			return out.Volumes, nil
		})
		return volume == nil || volume.State == types.VolumeStateAvailable, err
	})
	if err != nil || volume == nil {
		return err
	}
	_, err = r.ec2.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(id)})
	return awsReapErr(id, "DeleteVolume", err)
}

func describeAWSNetworkInterface(ctx context.Context, r awsReaper, id string) (*types.NetworkInterface, error) {
	return awsOne(id, func(n types.NetworkInterface) *string { return n.NetworkInterfaceId }, func() ([]types.NetworkInterface, error) {
		out, err := r.ec2.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{NetworkInterfaceIds: []string{id}})
		if err != nil {
			return nil, err
		}
		return out.NetworkInterfaces, nil
	})
}

// A network interface is detached if attached, and deleted once
// available: the detach is asynchronous.
func deleteAWSNetworkInterface(ctx context.Context, r awsReaper, id string) error {
	eni, err := describeAWSNetworkInterface(ctx, r, id)
	if err != nil || eni == nil {
		return awsReapErr(id, "DescribeNetworkInterfaces", err)
	}
	if eni.Status == types.NetworkInterfaceStatusInUse && eni.Attachment != nil {
		_, err := r.ec2.DetachNetworkInterface(ctx, &ec2.DetachNetworkInterfaceInput{AttachmentId: eni.Attachment.AttachmentId})
		if err := awsReapErr(id, "DetachNetworkInterface", err); err != nil {
			return err
		}
	}
	err = r.settle(ctx, id, "available", func() (bool, error) {
		var err error
		eni, err = describeAWSNetworkInterface(ctx, r, id)
		return eni == nil || eni.Status == types.NetworkInterfaceStatusAvailable, err
	})
	if err != nil || eni == nil {
		return err
	}
	_, err = r.ec2.DeleteNetworkInterface(ctx, &ec2.DeleteNetworkInterfaceInput{NetworkInterfaceId: aws.String(id)})
	return awsReapErr(id, "DeleteNetworkInterface", err)
}

func deleteAWSKeyPair(ctx context.Context, r awsReaper, id string) error {
	_, err := r.ec2.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{KeyPairId: aws.String(id)})
	return awsReapErr(id, "DeleteKeyPair", err)
}

// revokeAWSGroupRules revokes every rule the group holds, passing them
// back as DescribeSecurityGroups reports them.
func revokeAWSGroupRules(ctx context.Context, r awsReaper, id string) error {
	group, err := awsOne(id, func(g types.SecurityGroup) *string { return g.GroupId }, func() ([]types.SecurityGroup, error) {
		out, err := r.ec2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{GroupIds: []string{id}})
		if err != nil {
			return nil, err
		}
		return out.SecurityGroups, nil
	})
	if err != nil || group == nil {
		return awsReapErr(id, "DescribeSecurityGroups", err)
	}
	if len(group.IpPermissions) > 0 {
		_, err := r.ec2.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{GroupId: aws.String(id), IpPermissions: group.IpPermissions})
		if err := awsReapErr(id, "RevokeSecurityGroupIngress", err); err != nil {
			return err
		}
	}
	if len(group.IpPermissionsEgress) > 0 {
		_, err := r.ec2.RevokeSecurityGroupEgress(ctx, &ec2.RevokeSecurityGroupEgressInput{GroupId: aws.String(id), IpPermissions: group.IpPermissionsEgress})
		return awsReapErr(id, "RevokeSecurityGroupEgress", err)
	}
	return nil
}

func deleteAWSSecurityGroup(ctx context.Context, r awsReaper, id string) error {
	_, err := r.ec2.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{GroupId: aws.String(id)})
	return awsReapErr(id, "DeleteSecurityGroup", err)
}

func deleteAWSRouteTable(ctx context.Context, r awsReaper, id string) error {
	table, err := awsOne(id, func(t types.RouteTable) *string { return t.RouteTableId }, func() ([]types.RouteTable, error) {
		out, err := r.ec2.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{RouteTableIds: []string{id}})
		if err != nil {
			return nil, err
		}
		return out.RouteTables, nil
	})
	if err != nil || table == nil {
		return awsReapErr(id, "DescribeRouteTables", err)
	}
	for _, association := range table.Associations {
		if aws.ToBool(association.Main) {
			continue
		}
		_, err := r.ec2.DisassociateRouteTable(ctx, &ec2.DisassociateRouteTableInput{AssociationId: association.RouteTableAssociationId})
		if err := awsReapErr(id, "DisassociateRouteTable", err); err != nil {
			return err
		}
	}
	_, err = r.ec2.DeleteRouteTable(ctx, &ec2.DeleteRouteTableInput{RouteTableId: aws.String(id)})
	return awsReapErr(id, "DeleteRouteTable", err)
}

func deleteAWSSubnet(ctx context.Context, r awsReaper, id string) error {
	_, err := r.ec2.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: aws.String(id)})
	return awsReapErr(id, "DeleteSubnet", err)
}

func deleteAWSInternetGateway(ctx context.Context, r awsReaper, id string) error {
	gateway, err := awsOne(id, func(g types.InternetGateway) *string { return g.InternetGatewayId }, func() ([]types.InternetGateway, error) {
		out, err := r.ec2.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{InternetGatewayIds: []string{id}})
		if err != nil {
			return nil, err
		}
		return out.InternetGateways, nil
	})
	if err != nil || gateway == nil {
		return awsReapErr(id, "DescribeInternetGateways", err)
	}
	for _, attachment := range gateway.Attachments {
		_, err := r.ec2.DetachInternetGateway(ctx, &ec2.DetachInternetGatewayInput{InternetGatewayId: aws.String(id), VpcId: attachment.VpcId})
		if err := awsReapErr(id, "DetachInternetGateway", err); err != nil {
			return err
		}
	}
	_, err = r.ec2.DeleteInternetGateway(ctx, &ec2.DeleteInternetGatewayInput{InternetGatewayId: aws.String(id)})
	return awsReapErr(id, "DeleteInternetGateway", err)
}

func deleteAWSVPC(ctx context.Context, r awsReaper, id string) error {
	_, err := r.ec2.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: aws.String(id)})
	return awsReapErr(id, "DeleteVpc", err)
}
