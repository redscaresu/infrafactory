package e2e

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// reapRecorder carries EC2 requests to fakeaws, keeping the status each
// action answered. before runs on each action before it is sent.
type reapRecorder struct {
	inner  *http.Client
	before func(action string)

	mu       sync.Mutex
	statuses map[string][]int
}

func (r *reapRecorder) Do(req *http.Request) (*http.Response, error) {
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(payload))
	form, err := neturl.ParseQuery(string(payload))
	if err != nil {
		return nil, err
	}
	action := form.Get("Action")
	r.before(action)
	resp, err := r.inner.Do(req)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses[action] = append(r.statuses[action], resp.StatusCode)
	return resp, nil
}

// TestAWSReapAgainstFakeaws runs the reap through the real sealed env and
// SDK clients against fakeaws STS, SSM and EC2, over a planted step-one
// stack. fakeaws proves each request decodes and is answered; it does
// not prove the order, which it does not enforce
// (TestEveryAWSSweptCollectionHasAReapDelete holds that). Its DeleteVpc
// cascades to what the VPC holds, so the test proves the subnet, both
// groups and the route table gone by their own deletes before DeleteVpc
// is sent. The required CI job runs it by name and fails if it does not
// report PASS.
func TestAWSReapAgainstFakeaws(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const region = "eu-west-2"
	endpoints := harness.AWSEndpoints{
		EC2: mock.URL + "/ec2/region/" + region,
		SSM: mock.URL + "/ssm/region/" + region,
		STS: mock.URL + "/sts",
	}

	credFile := filepath.Join(t.TempDir(), "layer3-aws.env")
	require.NoError(t, os.WriteFile(credFile, []byte("AWS_ACCESS_KEY_ID=fakeaws-key-id\nAWS_SECRET_ACCESS_KEY=fakeaws-secret\n"), 0o600))
	env, err := harness.AWSSealedEnv(credFile, region)
	require.NoError(t, err)
	doer := harness.NewLoopbackOnlyHTTPClient()
	ctx := context.Background()
	creds := credentials.NewStaticCredentialsProvider("fakeaws-key-id", "fakeaws-secret", "")
	ec2Client := ec2.New(ec2.Options{Region: region, Credentials: creds, BaseEndpoint: aws.String(endpoints.EC2), HTTPClient: doer})
	ssmClient := ssm.New(ssm.Options{Region: region, Credentials: creds, BaseEndpoint: aws.String(endpoints.SSM), HTTPClient: doer})

	_, err = ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name: aws.String(harness.AWSStampParameter), Value: aws.String(fakeawsAccount), Type: ssmtypes.ParameterTypeString,
	})
	require.NoError(t, err)
	holder, err := harness.NewAWSClaimHolder("e2e-reap")
	require.NoError(t, err)
	require.NoError(t, harness.TakeAWSClaim(ctx, env, doer, endpoints.SSM, holder))

	vpc, err := ec2Client.CreateVpc(ctx, &ec2.CreateVpcInput{CidrBlock: aws.String("10.0.0.0/16")})
	require.NoError(t, err)
	vpcID := vpc.Vpc.VpcId
	subnet, err := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{VpcId: vpcID, CidrBlock: aws.String("10.0.1.0/24")})
	require.NoError(t, err)
	subnetID := subnet.Subnet.SubnetId
	igw, err := ec2Client.CreateInternetGateway(ctx, &ec2.CreateInternetGatewayInput{})
	require.NoError(t, err)
	igwID := igw.InternetGateway.InternetGatewayId
	_, err = ec2Client.AttachInternetGateway(ctx, &ec2.AttachInternetGatewayInput{InternetGatewayId: igwID, VpcId: vpcID})
	require.NoError(t, err)
	table, err := ec2Client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{VpcId: vpcID})
	require.NoError(t, err)
	tableID := table.RouteTable.RouteTableId
	_, err = ec2Client.AssociateRouteTable(ctx, &ec2.AssociateRouteTableInput{RouteTableId: tableID, SubnetId: subnetID})
	require.NoError(t, err)
	_, err = ec2Client.CreateRoute(ctx, &ec2.CreateRouteInput{RouteTableId: tableID, DestinationCidrBlock: aws.String("0.0.0.0/0"), GatewayId: igwID})
	require.NoError(t, err)
	var groupIDs []*string
	for _, name := range []string{"lb", "web"} {
		group, err := ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{GroupName: aws.String(name), Description: aws.String(name), VpcId: vpcID})
		require.NoError(t, err)
		groupIDs = append(groupIDs, group.GroupId)
	}
	// web admits lb, so lb cannot go while web's rule names it.
	_, err = ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: groupIDs[1],
		IpPermissions: []ec2types.IpPermission{{
			IpProtocol: aws.String("tcp"), FromPort: aws.Int32(80), ToPort: aws.Int32(80),
			UserIdGroupPairs: []ec2types.UserIdGroupPair{{GroupId: groupIDs[0]}},
		}},
	})
	require.NoError(t, err)
	_, err = ec2Client.ImportKeyPair(ctx, &ec2.ImportKeyPairInput{
		KeyName: aws.String("stray"), PublicKeyMaterial: []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGNvbnRlbnQtb2YtYS10ZXN0LWtleS0wMDAwMDAwMA stray"),
	})
	require.NoError(t, err)
	_, err = ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{Domain: ec2types.DomainTypeVpc})
	require.NoError(t, err)
	_, err = ec2Client.RunInstances(ctx, &ec2.RunInstancesInput{
		ImageId: aws.String("ami-0al2023x8664"), InstanceType: ec2types.InstanceTypeT3Micro,
		SubnetId: subnetID, MinCount: aws.Int32(1), MaxCount: aws.Int32(1),
	})
	require.NoError(t, err)

	sweepDoers := harness.AWSDoers{EC2: doer, SSM: doer}
	strays, err := harness.SweepAWSScope(ctx, env, sweepDoers, endpoints, nil)
	require.Error(t, err, "planted")

	// A Describe by id of each thing DeleteVpc would cascade to, just
	// before it is sent.
	gone := map[string]error{}
	recorder := &reapRecorder{inner: doer, statuses: map[string][]int{}, before: func(action string) {
		if action != "DeleteVpc" {
			return
		}
		subnets, err := ec2Client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{SubnetIds: []string{*subnetID}})
		gone["subnet"] = describedGone(err, subnets != nil && len(subnets.Subnets) > 0)
		for i, id := range groupIDs {
			groups, err := ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{GroupIds: []string{*id}})
			gone["group "+[]string{"lb", "web"}[i]] = describedGone(err, groups != nil && len(groups.SecurityGroups) > 0)
		}
		tables, err := ec2Client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{RouteTableIds: []string{*tableID}})
		gone["route table"] = describedGone(err, tables != nil && len(tables.RouteTables) > 0)
	}}

	err = harness.ReapAWSScope(ctx, env, harness.AWSDoers{EC2: recorder, SSM: doer, STS: doer}, endpoints, nil,
		fakeawsAccount, fakeawsPrincipal, holder, strays)
	require.NoError(t, err, "reap")

	for _, action := range []string{
		"TerminateInstances", "ReleaseAddress", "DeleteKeyPair", "RevokeSecurityGroupIngress", "DeleteSecurityGroup",
		"DeleteSubnet", "DisassociateRouteTable", "DeleteRouteTable", "DetachInternetGateway", "DeleteInternetGateway", "DeleteVpc",
	} {
		statuses := recorder.statuses[action]
		assert.NotEmpty(t, statuses, "%s was not sent", action)
		for _, status := range statuses {
			assert.Equal(t, http.StatusOK, status, action)
		}
	}
	for _, what := range []string{"subnet", "group lb", "group web", "route table"} {
		err, checked := gone[what]
		require.True(t, checked, "DeleteVpc was sent before %s was checked", what)
		assert.NoError(t, err, "%s before DeleteVpc", what)
	}

	strays, err = harness.SweepAWSScope(ctx, env, sweepDoers, endpoints, nil)
	require.NoError(t, err, "after the reap")
	assert.Empty(t, strays)
}

// describedGone is nil when a Describe by id says the item is gone: a
// NotFound, or an answer without it. fakeaws answers an unknown subnet
// with an empty set where AWS says InvalidSubnetID.NotFound.
func describedGone(err error, found bool) error {
	var apiErr smithy.APIError
	switch {
	case errors.As(err, &apiErr) && strings.HasSuffix(apiErr.ErrorCode(), "NotFound"):
		return nil
	case err != nil:
		return err
	case found:
		return errors.New("still there")
	}
	return nil
}
