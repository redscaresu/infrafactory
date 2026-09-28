package harness

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// AWSRunIDTagKey is the tag default_tags puts on every AWS resource a run
// applies, valued with the run id. The sweep prints it beside a stray and
// never gates on it: an untagged stray is as much a leak as a tagged one.
const AWSRunIDTagKey = "infrafactory-run-id"

// The settle loop re-polls the whole scope every awsSweepSettleInterval
// while anything is still going away, and fails after AWSSweepMaxPolls.
// Ten minutes covers a NAT gateway, the slowest to reach deleted.
const (
	AWSSweepMaxPolls       = 60
	awsSweepSettleInterval = 10 * time.Second
)

// awsSettlingStates are the states of something on its way out. While
// any item is in one, the sweep polls again rather than failing.
var awsSettlingStates = map[string]bool{"shutting-down": true, "deleting": true, "detaching": true}

// AWSDoers carry the sweep's requests, one per service.
type AWSDoers struct {
	EC2 ec2.HTTPClient
	SSM ssm.HTTPClient
}

// AWSEndpoints are the services' endpoints; "" means real AWS in the
// sealed env's region.
type AWSEndpoints struct {
	EC2, SSM string
}

// AWSStray is one thing the sweep found in the scope. RunID is its
// AWSRunIDTagKey tag, "" when it has none.
type AWSStray struct {
	Collection, ID, State, RunID string
}

func (s AWSStray) String() string {
	var detail []string
	if s.State != "" {
		detail = append(detail, s.State)
	}
	if s.RunID != "" {
		detail = append(detail, AWSRunIDTagKey+"="+s.RunID)
	}
	if len(detail) == 0 {
		return s.Collection + " " + s.ID
	}
	return fmt.Sprintf("%s %s (%s)", s.Collection, s.ID, strings.Join(detail, ", "))
}

// AWSSweepCollection is one row of the sweep: a collection in the
// scope's region, and what in it is not a leak.
type AWSSweepCollection struct {
	Name string
	list func(context.Context, awsSweepClients) ([]AWSStray, error)
}

type awsSweepClients struct {
	ec2 *ec2.Client
	ssm *ssm.Client
}

// AWSSweepCollections is the scope's fixed collection list (HLD
// 2026-09-27-aws-web-stack § Containment), independent of the
// allowlist. Empty means every row lists nothing.
var AWSSweepCollections = []AWSSweepCollection{
	{"instances", listAWSInstances},
	{"volumes", listAWSVolumes},
	{"network interfaces", listAWSNetworkInterfaces},
	{"elastic ips", listAWSAddresses},
	{"security groups", listAWSSecurityGroups},
	{"vpcs", listAWSVPCs},
	{"subnets", listAWSSubnets},
	{"internet gateways", listAWSInternetGateways},
	{"route tables", listAWSRouteTables},
	{"nat gateways", listAWSNATGateways},
	{"key pairs", listAWSKeyPairs},
	{"images", listAWSImages},
	{"snapshots", listAWSSnapshots},
	{"launch templates", listAWSLaunchTemplates},
	{"ssm parameters", listAWSParameters},
}

// SweepAWSScope proves the scope empty, or fails naming what is in it.
// Each poll lists every collection. While anything is shutting-down,
// deleting or detaching, it sleeps and polls again, up to
// AWSSweepMaxPolls. The strays it returns are the last poll's, and the
// error is nil only when that poll found none. A collection that cannot
// be listed in full fails the sweep and returns no strays. sleep nil
// means a real sleep.
func SweepAWSScope(ctx context.Context, env map[string]string, doers AWSDoers, endpoints AWSEndpoints, sleep func(context.Context, time.Duration) error) ([]AWSStray, error) {
	ec2Client, err := newAWSEC2Client(env, doers.EC2, endpoints.EC2)
	if err != nil {
		return nil, fmt.Errorf("aws scope sweep: %w", err)
	}
	ssmClient, err := newAWSSSMClient(env, doers.SSM, endpoints.SSM)
	if err != nil {
		return nil, fmt.Errorf("aws scope sweep: %w", err)
	}
	if sleep == nil {
		sleep = sleepContext
	}
	clients := awsSweepClients{ec2: ec2Client, ssm: ssmClient}

	for poll := 1; ; poll++ {
		strays, err := pollAWSScope(ctx, clients)
		if err != nil {
			return nil, err
		}
		if !awsScopeSettling(strays) {
			if len(strays) == 0 {
				return nil, nil
			}
			return strays, fmt.Errorf("aws scope sweep: the scope is not empty: %s", joinAWSStrays(strays))
		}
		if poll == AWSSweepMaxPolls {
			return strays, fmt.Errorf("aws scope sweep: still settling after %d polls: %s", poll, joinAWSStrays(strays))
		}
		if err := sleep(ctx, awsSweepSettleInterval); err != nil {
			return nil, fmt.Errorf("aws scope sweep: waiting to poll again: %w", err)
		}
	}
}

func pollAWSScope(ctx context.Context, clients awsSweepClients) ([]AWSStray, error) {
	var strays []AWSStray
	for _, row := range AWSSweepCollections {
		found, err := row.list(ctx, clients)
		if err != nil {
			return nil, fmt.Errorf("aws scope sweep: %s: %w", row.Name, err)
		}
		for _, stray := range found {
			stray.Collection = row.Name
			strays = append(strays, stray)
		}
	}
	return strays, nil
}

func awsScopeSettling(strays []AWSStray) bool {
	for _, stray := range strays {
		if awsSettlingStates[stray.State] {
			return true
		}
	}
	return false
}

func joinAWSStrays(strays []AWSStray) string {
	names := make([]string, len(strays))
	for i, stray := range strays {
		names[i] = stray.String()
	}
	return strings.Join(names, "; ")
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// awsPages calls page with each NextToken in turn, from none until one
// comes back empty. A token seen before fails: the listing cycles and
// would never be complete. The SDK paginators' StopOnDuplicateToken
// would end it silently instead, as though it were.
func awsPages(page func(token *string) (next *string, err error)) error {
	seen := map[string]bool{}
	var token *string
	for {
		next, err := page(token)
		if err != nil {
			return err
		}
		if aws.ToString(next) == "" {
			return nil
		}
		if seen[*next] {
			return fmt.Errorf("NextToken %q came back a second time, so the listing cycles and is incomplete", *next)
		}
		seen[*next] = true
		token = next
	}
}

func awsRunIDTag(tags []types.Tag) string {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == AWSRunIDTagKey {
			return aws.ToString(tag.Value)
		}
	}
	return ""
}

// A terminated instance stays listed for about an hour, and is gone.
func listAWSInstances(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, reservation := range out.Reservations {
			for _, instance := range reservation.Instances {
				var state string
				if instance.State != nil {
					state = string(instance.State.Name)
				}
				if state == string(types.InstanceStateNameTerminated) {
					continue
				}
				found = append(found, AWSStray{ID: aws.ToString(instance.InstanceId), State: state, RunID: awsRunIDTag(instance.Tags)})
			}
		}
		return out.NextToken, nil
	})
	return found, err
}

// A volume has no detaching state of its own; its attachment does.
func listAWSVolumes(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, volume := range out.Volumes {
			state := string(volume.State)
			for _, attachment := range volume.Attachments {
				if attachment.State == types.VolumeAttachmentStateDetaching {
					state = string(attachment.State)
				}
			}
			found = append(found, AWSStray{ID: aws.ToString(volume.VolumeId), State: state, RunID: awsRunIDTag(volume.Tags)})
		}
		return out.NextToken, nil
	})
	return found, err
}

func listAWSNetworkInterfaces(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, eni := range out.NetworkInterfaces {
			found = append(found, AWSStray{ID: aws.ToString(eni.NetworkInterfaceId), State: string(eni.Status), RunID: awsRunIDTag(eni.TagSet)})
		}
		return out.NextToken, nil
	})
	return found, err
}

// DescribeAddresses does not paginate: one call lists every address.
func listAWSAddresses(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	out, err := c.ec2.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
	if err != nil {
		return nil, err
	}
	var found []AWSStray
	for _, address := range out.Addresses {
		found = append(found, AWSStray{ID: aws.ToString(address.AllocationId), RunID: awsRunIDTag(address.Tags)})
	}
	return found, nil
}

// Every VPC has a group named default, which cannot be deleted and goes
// with its VPC; no other group may take the name.
func listAWSSecurityGroups(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, group := range out.SecurityGroups {
			if aws.ToString(group.GroupName) == "default" {
				continue
			}
			found = append(found, AWSStray{ID: aws.ToString(group.GroupId), RunID: awsRunIDTag(group.Tags)})
		}
		return out.NextToken, nil
	})
	return found, err
}

// Every VPC is a stray, the default one included: setup deletes it.
func listAWSVPCs(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, vpc := range out.Vpcs {
			found = append(found, AWSStray{ID: aws.ToString(vpc.VpcId), State: string(vpc.State), RunID: awsRunIDTag(vpc.Tags)})
		}
		return out.NextToken, nil
	})
	return found, err
}

func listAWSSubnets(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, subnet := range out.Subnets {
			found = append(found, AWSStray{ID: aws.ToString(subnet.SubnetId), State: string(subnet.State), RunID: awsRunIDTag(subnet.Tags)})
		}
		return out.NextToken, nil
	})
	return found, err
}

// A gateway's only state is its attachment's.
func listAWSInternetGateways(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, gateway := range out.InternetGateways {
			var state string
			for _, attachment := range gateway.Attachments {
				state = string(attachment.State)
			}
			found = append(found, AWSStray{ID: aws.ToString(gateway.InternetGatewayId), State: state, RunID: awsRunIDTag(gateway.Tags)})
		}
		return out.NextToken, nil
	})
	return found, err
}

// A VPC's main route table cannot be deleted and goes with its VPC.
func listAWSRouteTables(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, table := range out.RouteTables {
			if awsMainRouteTable(table) {
				continue
			}
			found = append(found, AWSStray{ID: aws.ToString(table.RouteTableId), RunID: awsRunIDTag(table.Tags)})
		}
		return out.NextToken, nil
	})
	return found, err
}

func awsMainRouteTable(table types.RouteTable) bool {
	for _, association := range table.Associations {
		if aws.ToBool(association.Main) {
			return true
		}
	}
	return false
}

// A deleted NAT gateway stays listed for about an hour, and is gone.
func listAWSNATGateways(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeNatGateways(ctx, &ec2.DescribeNatGatewaysInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, gateway := range out.NatGateways {
			if gateway.State == types.NatGatewayStateDeleted {
				continue
			}
			found = append(found, AWSStray{ID: aws.ToString(gateway.NatGatewayId), State: string(gateway.State), RunID: awsRunIDTag(gateway.Tags)})
		}
		return out.NextToken, nil
	})
	return found, err
}

// DescribeKeyPairs does not paginate: one call lists every key pair.
func listAWSKeyPairs(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	out, err := c.ec2.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{})
	if err != nil {
		return nil, err
	}
	var found []AWSStray
	for _, key := range out.KeyPairs {
		found = append(found, AWSStray{ID: aws.ToString(key.KeyPairId), RunID: awsRunIDTag(key.Tags)})
	}
	return found, nil
}

// Owners=self lists only the account's own images, never the public ones
// it can launch; IncludeDisabled adds the disabled ones, which are
// otherwise hidden and still billed for their snapshots.
func listAWSImages(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeImages(ctx, &ec2.DescribeImagesInput{
			Owners: []string{"self"}, IncludeDisabled: aws.Bool(true), NextToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, image := range out.Images {
			found = append(found, AWSStray{ID: aws.ToString(image.ImageId), State: string(image.State), RunID: awsRunIDTag(image.Tags)})
		}
		return out.NextToken, nil
	})
	return found, err
}

// OwnerIds=self lists only the account's own snapshots, never the public
// ones.
func listAWSSnapshots(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{OwnerIds: []string{"self"}, NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, snapshot := range out.Snapshots {
			found = append(found, AWSStray{ID: aws.ToString(snapshot.SnapshotId), State: string(snapshot.State), RunID: awsRunIDTag(snapshot.Tags)})
		}
		return out.NextToken, nil
	})
	return found, err
}

func listAWSLaunchTemplates(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ec2.DescribeLaunchTemplates(ctx, &ec2.DescribeLaunchTemplatesInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, template := range out.LaunchTemplates {
			found = append(found, AWSStray{ID: aws.ToString(template.LaunchTemplateId), RunID: awsRunIDTag(template.Tags)})
		}
		return out.NextToken, nil
	})
	return found, err
}

// The claim and the stamp are the scope's own infrastructure. Any other
// parameter, under the prefix or not, is a stray.
func listAWSParameters(ctx context.Context, c awsSweepClients) ([]AWSStray, error) {
	var found []AWSStray
	err := awsPages(func(token *string) (*string, error) {
		out, err := c.ssm.DescribeParameters(ctx, &ssm.DescribeParametersInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, parameter := range out.Parameters {
			name := aws.ToString(parameter.Name)
			if name == AWSClaimParameter || name == AWSStampParameter {
				continue
			}
			found = append(found, AWSStray{ID: name})
		}
		return out.NextToken, nil
	})
	return found, err
}
