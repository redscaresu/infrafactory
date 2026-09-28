package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// TestAWSSweepAgainstFakeaws runs the scope sweep through the real sealed
// env and SDK clients against fakeaws EC2 and SSM: an empty scope holding
// only the stamp and the claim is clean, and each thing planted in it is
// named. The required CI job runs it by name and fails if it does not
// report PASS.
func TestAWSSweepAgainstFakeaws(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const region = "eu-west-2"
	endpoints := harness.AWSEndpoints{
		EC2: mock.URL + "/ec2/region/" + region,
		SSM: mock.URL + "/ssm/region/" + region,
	}

	credFile := filepath.Join(t.TempDir(), "layer3-aws.env")
	require.NoError(t, os.WriteFile(credFile, []byte("AWS_ACCESS_KEY_ID=fakeaws-key-id\nAWS_SECRET_ACCESS_KEY=fakeaws-secret\n"), 0o600))
	env, err := harness.AWSSealedEnv(credFile, region)
	require.NoError(t, err)
	doer := harness.NewLoopbackOnlyHTTPClient()
	doers := harness.AWSDoers{EC2: doer, SSM: doer}
	ctx := context.Background()
	creds := credentials.NewStaticCredentialsProvider("fakeaws-key-id", "fakeaws-secret", "")
	ec2Client := ec2.New(ec2.Options{Region: region, Credentials: creds, BaseEndpoint: aws.String(endpoints.EC2), HTTPClient: doer})
	ssmClient := ssm.New(ssm.Options{Region: region, Credentials: creds, BaseEndpoint: aws.String(endpoints.SSM), HTTPClient: doer})

	strays, err := harness.SweepAWSScope(ctx, env, doers, endpoints, nil)
	require.NoError(t, err, "empty")
	assert.Empty(t, strays)

	// The scope's own parameters are not strays.
	_, err = ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name: aws.String(harness.AWSStampParameter), Value: aws.String(fakeawsAccount), Type: ssmtypes.ParameterTypeString,
	})
	require.NoError(t, err)
	holder, err := harness.NewAWSClaimHolder("e2e-sweep")
	require.NoError(t, err)
	require.NoError(t, harness.TakeAWSClaim(ctx, env, doer, endpoints.SSM, holder))
	_, err = harness.SweepAWSScope(ctx, env, doers, endpoints, nil)
	require.NoError(t, err, "only the stamp and the claim")

	vpc, err := ec2Client.CreateVpc(ctx, &ec2.CreateVpcInput{CidrBlock: aws.String("10.0.0.0/16")})
	require.NoError(t, err)
	vpcID := aws.ToString(vpc.Vpc.VpcId)
	subnet, err := ec2Client.CreateSubnet(ctx, &ec2.CreateSubnetInput{VpcId: aws.String(vpcID), CidrBlock: aws.String("10.0.1.0/24")})
	require.NoError(t, err)
	subnetID := aws.ToString(subnet.Subnet.SubnetId)
	group, err := ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName: aws.String("web"), Description: aws.String("web"), VpcId: aws.String(vpcID),
	})
	require.NoError(t, err)
	key, err := ec2Client.ImportKeyPair(ctx, &ec2.ImportKeyPairInput{
		KeyName: aws.String("stray"), PublicKeyMaterial: []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGNvbnRlbnQtb2YtYS10ZXN0LWtleS0wMDAwMDAwMA stray"),
	})
	require.NoError(t, err)
	address, err := ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{Domain: ec2types.DomainTypeVpc})
	require.NoError(t, err)
	run, err := ec2Client.RunInstances(ctx, &ec2.RunInstancesInput{
		ImageId: aws.String("ami-0al2023x8664"), InstanceType: ec2types.InstanceTypeT3Micro,
		SubnetId: aws.String(subnetID), MinCount: aws.Int32(1), MaxCount: aws.Int32(1),
	})
	require.NoError(t, err)
	require.Len(t, run.Instances, 1)
	_, err = ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name: aws.String("/other/x"), Value: aws.String("x"), Type: ssmtypes.ParameterTypeString,
	})
	require.NoError(t, err)

	_, err = harness.SweepAWSScope(ctx, env, doers, endpoints, nil)
	require.Error(t, err, "planted")
	for _, want := range []string{
		"vpcs " + vpcID,
		"subnets " + subnetID,
		"security groups " + aws.ToString(group.GroupId),
		"key pairs " + aws.ToString(key.KeyPairId),
		"elastic ips " + aws.ToString(address.AllocationId),
		"instances " + aws.ToString(run.Instances[0].InstanceId),
		"ssm parameters /other/x",
	} {
		assert.Contains(t, err.Error(), want)
	}
	assert.NotContains(t, err.Error(), harness.AWSStampParameter)
	assert.NotContains(t, err.Error(), harness.AWSClaimParameter)
}
