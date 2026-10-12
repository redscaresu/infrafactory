package e2e

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// awsStep0ReadSet is the wedge's step-0 read set (ADR-0041): the
// requests the lead replays against the real account, so it holds no id.
const awsStep0ReadSet = "testdata/aws-step0/read-set.json"

// The entries of awsStep0ReadSet, by index, that the tests name.
const (
	step0VPCsByTag = iota
	step0Zones
	step0GroupsByTag
	step0AbsentGroup
	step0AMIParameter
)

// fakeawsStep0PinRefusedRows are the EC2 follow-up rows fakeaws at
// FAKEAWS_SHA refuses in TestAWSReadStep0AgainstFakeaws (the rows are
// explained at fakeawsEC2PinRefusedRows).
var fakeawsStep0PinRefusedRows = []string{"DescribeSecurityGroupRules group-id"}

// step0AbsentGroupName is the SG name step0AbsentGroup looks up in the
// tagged VPC, where it does not exist.
const step0AbsentGroupName = "infrafactory-absent"

// awsStep0Leaks is what the read set must never hold: an account id, a
// resource id (any prefix: vpc-, sg-, rtb-, ami-, eipalloc-, ...), a run id (run_command.go's 20060102T150405Z0700 stem, in either
// case) or a GroupName.N key, which AWS resolves only in the default
// VPC.
var awsStep0Leaks = map[string]*regexp.Regexp{
	"a 12-digit number": regexp.MustCompile(`\d{12}`),
	"a resource id":     regexp.MustCompile(`\b[a-z]+-[0-9a-f]{8,17}\b`),
	"a run-id value":    regexp.MustCompile(`(?i)\d{8}t\d{6}`),
	"a GroupName.N key": regexp.MustCompile(`"GroupName\.`),
}

// loadAWSStep0ReadSet decodes the read set as `rehearse read --read-set`
// does (harness.DecodeAWSReadSet).
func loadAWSStep0ReadSet(t *testing.T) []harness.AWSReadEntry {
	t.Helper()
	data, err := os.ReadFile(awsStep0ReadSet)
	require.NoError(t, err)
	entries, err := harness.DecodeAWSReadSet(data)
	require.NoError(t, err)
	return entries
}

// TestAWSStep0ReadSetHoldsNoIDs fails when the committed read set holds
// anything awsStep0Leaks names, an entry outside the reader table, or
// any entry (service, protocol, action, params) other than its own, with
// the harness's tag key and parameter name.
func TestAWSStep0ReadSetHoldsNoIDs(t *testing.T) {
	data, err := os.ReadFile(awsStep0ReadSet)
	require.NoError(t, err)
	for what, re := range awsStep0Leaks {
		assert.Empty(t, re.FindAllString(string(data), -1), "the read set holds %s", what)
	}

	entries := loadAWSStep0ReadSet(t)
	require.NoError(t, harness.ValidateAWSReadSet(entries))
	byTag := `{"Filter.1.Name":"tag-key","Filter.1.Value.1":"` + harness.AWSRunIDTagKey + `"}`
	ec2Query := func(action, params string) harness.AWSReadEntry {
		return harness.AWSReadEntry{Service: "ec2", Protocol: "query", Action: action, Params: json.RawMessage(params)}
	}
	want := map[int]harness.AWSReadEntry{
		step0VPCsByTag:    ec2Query("DescribeVpcs", byTag),
		step0Zones:        ec2Query("DescribeAvailabilityZones", `{}`),
		step0GroupsByTag:  ec2Query("DescribeSecurityGroups", byTag),
		step0AbsentGroup:  ec2Query("DescribeSecurityGroups", `{"Filter.1.Name":"group-name","Filter.1.Value.1":"`+step0AbsentGroupName+`","Filter.2.Name":"tag-key","Filter.2.Value.1":"`+harness.AWSRunIDTagKey+`"}`),
		step0AMIParameter: {Service: "ssm", Protocol: "json", Action: "GetParameter", Params: json.RawMessage(`{"Name":"` + harness.AWSAL2023AMIParameter + `"}`)},
	}
	require.Len(t, entries, len(want))
	for i, entry := range want {
		wantJSON, err := json.Marshal(entry)
		require.NoError(t, err)
		gotJSON, err := json.Marshal(entries[i])
		require.NoError(t, err)
		assert.JSONEq(t, string(wantJSON), string(gotJSON), "entry %d", i)
	}
}

// TestAWSReadStep0AgainstFakeaws reads the committed step-0 read set
// through the walls against fakeaws, seeded with one VPC tagged
// AWSRunIDTagKey (with its subnet and SG) and one untagged VPC beside it,
// whose SGs include an untagged one named step0AbsentGroupName.
// Each entry is answered at the pin, filters and zones included, and so
// is each follow-up but DescribeSecurityGroupRules (fakeawsStep0PinRefusedRows).
// The required CI job runs it by name and fails if it does not report PASS.
func TestAWSReadStep0AgainstFakeaws(t *testing.T) {
	SkipUnlessEnabled(t)
	mock := StartFakeaws(t)
	const region = "eu-west-2"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	credFile := filepath.Join(t.TempDir(), "reader-aws.env")
	require.NoError(t, os.WriteFile(credFile, []byte("AWS_ACCESS_KEY_ID=fakeaws-key-id\nAWS_SECRET_ACCESS_KEY=fakeaws-secret\n"), 0o600))
	cfg, err := harness.AWSReaderConfig(credFile, region)
	require.NoError(t, err)

	// Planted unwalled: the read itself sends only reader-table actions.
	creds := credentials.NewStaticCredentialsProvider("fakeaws-key-id", "fakeaws-secret", "")
	loopback := harness.NewLoopbackOnlyHTTPClient()
	ec2Client := ec2.New(ec2.Options{Region: region, Credentials: creds, BaseEndpoint: aws.String(mock.URL + "/ec2/region/" + region), HTTPClient: loopback})
	tagged := seedStep0VPC(ctx, t, ec2Client, true)
	untagged := seedStep0VPC(ctx, t, ec2Client, false)
	// Found only if fakeaws drops step0AbsentGroup's tag-key filter.
	_, err = ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		VpcId: aws.String(untagged.vpc), GroupName: aws.String(step0AbsentGroupName), Description: aws.String("untagged"),
	})
	require.NoError(t, err)

	readSet := loadAWSStep0ReadSet(t)
	doer := &pinGapDoer{inner: loopback, refused: map[string]bool{}, unfiltered: map[string]bool{}}
	read, err := harness.ReadAWSAccount(ctx, harness.AWSReadInput{
		Config: cfg, Account: fakeawsAccount, Principal: fakeawsPrincipal,
		ReadSet: readSet, Doer: doer,
		Identity:  harness.VerifyAWSReadIdentity,
		Preflight: func(context.Context, aws.Config, *harness.AWSReadWall) error { return nil },
		Endpoints: map[string]string{
			"sts": mock.URL + "/sts",
			"ssm": mock.URL + "/ssm/region/" + region,
			"ec2": mock.URL + "/ec2/region/" + region,
			"iam": mock.URL + "/iam",
		},
	})
	require.NoError(t, err)

	assert.ElementsMatch(t, fakeawsStep0PinRefusedRows, slices.Collect(maps.Keys(doer.refused)), "the rows fakeaws refuses at the pin")
	assert.ElementsMatch(t, fakeawsPinUnfilteredRows, slices.Collect(maps.Keys(doer.unfiltered)), "the rows whose filter fakeaws ignores at the pin")

	// Each entry is one page, so the replay's exchanges come first, in order.
	require.GreaterOrEqual(t, len(read.Exchanges), len(readSet))
	for i, entry := range readSet {
		assert.Equal(t, entry, read.Exchanges[i].AWSReadEntry, "entry %d has its exchange", i)
	}
	for _, exchange := range read.Exchanges {
		assert.Equal(t, http.StatusOK, exchange.Status, "%s %s: %s", exchange.Action, exchange.Params, exchange.Body)
	}
	groupIDs := regexp.MustCompile(`<groupId>([^<]*)</groupId>`)
	assert.Equal(t, [][]string{{"<vpcId>" + tagged.vpc + "</vpcId>", tagged.vpc}}, answerVPCIDs.FindAllStringSubmatch(read.Exchanges[step0VPCsByTag].Body, -1), "the VPC by tag, and only it")
	assert.Equal(t, [][]string{{"<groupId>" + tagged.sg + "</groupId>", tagged.sg}}, groupIDs.FindAllStringSubmatch(read.Exchanges[step0GroupsByTag].Body, -1), "the SG by tag, and only it")
	assert.NotContains(t, read.Exchanges[step0AbsentGroup].Body, "<groupId>", "the absent name finds no group")
	assert.Contains(t, read.Exchanges[step0Zones].Body, "<zoneName>"+region+"a</zoneName>")
	assert.Contains(t, read.Exchanges[step0AMIParameter].Body, harness.AWSLayer2AMI)

	// The follow-ups the old pin refused (DescribeAvailabilityZones is the
	// entry above), and the seeded subnet, each answered for the tagged VPC.
	byRow := map[string][]harness.AWSReadExchange{}
	for _, exchange := range read.Exchanges[len(readSet):] {
		byRow[exchange.Action] = append(byRow[exchange.Action], exchange)
	}
	assert.True(t, anyExchange(byRow["DescribeSecurityGroups"], tagged.vpc, "<groupId>"+tagged.sg+"</groupId>"), "SGs of %s", tagged.vpc)
	assert.True(t, anyExchange(byRow["DescribeNetworkInterfaces"], tagged.vpc, "<networkInterfaceSet"), "interfaces of %s, answered", tagged.vpc)
	assert.True(t, anyExchange(byRow["DescribeSubnets"], tagged.vpc, "<subnetId>"+tagged.subnet+"</subnetId>"), "subnets of %s", tagged.vpc)
	for _, exchange := range read.Exchanges {
		assert.NotContains(t, string(exchange.Params)+exchange.Body, untagged.vpc, "the untagged VPC was read: %s", exchange.Action)
	}
}

// seededStep0VPC is one VPC fakeaws holds, with its subnet and SG.
type seededStep0VPC struct{ vpc, subnet, sg string }

// seedStep0VPC creates a VPC with a subnet and an SG, the VPC and SG
// tagged AWSRunIDTagKey when tagged is set.
func seedStep0VPC(ctx context.Context, t *testing.T, c *ec2.Client, tagged bool) seededStep0VPC {
	t.Helper()
	tags := func(resource ec2types.ResourceType) []ec2types.TagSpecification {
		if !tagged {
			return nil
		}
		return []ec2types.TagSpecification{{ResourceType: resource, Tags: []ec2types.Tag{{Key: aws.String(harness.AWSRunIDTagKey), Value: aws.String("seeded-run")}}}}
	}
	vpc, err := c.CreateVpc(ctx, &ec2.CreateVpcInput{CidrBlock: aws.String("10.0.0.0/16"), TagSpecifications: tags(ec2types.ResourceTypeVpc)})
	require.NoError(t, err)
	subnet, err := c.CreateSubnet(ctx, &ec2.CreateSubnetInput{VpcId: vpc.Vpc.VpcId, CidrBlock: aws.String("10.0.1.0/24")})
	require.NoError(t, err)
	sg, err := c.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		VpcId: vpc.Vpc.VpcId, GroupName: aws.String("web"), Description: aws.String("web"),
		TagSpecifications: tags(ec2types.ResourceTypeSecurityGroup),
	})
	require.NoError(t, err)
	return seededStep0VPC{vpc: aws.ToString(vpc.Vpc.VpcId), subnet: aws.ToString(subnet.Subnet.SubnetId), sg: aws.ToString(sg.GroupId)}
}
