package e2e

import (
	"bytes"
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

// awsStep0Leaks is what the read set must never hold: an account id, a
// resource id, a run id (run_command.go's 20060102T150405Z0700 stem, in either
// case) or a GroupName.N key, which AWS resolves only in the default
// VPC.
var awsStep0Leaks = map[string]*regexp.Regexp{
	"a 12-digit number": regexp.MustCompile(`\d{12}`),
	"a resource id":     regexp.MustCompile(`\b(vpc|sg|subnet|i)-[0-9a-f]+\b`),
	"a run-id value":    regexp.MustCompile(`(?i)\d{8}t\d{6}`),
	"a GroupName.N key": regexp.MustCompile(`"GroupName\.`),
}

// loadAWSStep0ReadSet decodes the read set as `rehearse read --read-set`
// does: one JSON array of entries, no unknown field.
func loadAWSStep0ReadSet(t *testing.T) []harness.AWSReadEntry {
	t.Helper()
	data, err := os.ReadFile(awsStep0ReadSet)
	require.NoError(t, err)
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var entries []harness.AWSReadEntry
	require.NoError(t, dec.Decode(&entries))
	return entries
}

// TestAWSStep0ReadSetHoldsNoIDs fails when the committed read set holds
// anything awsStep0Leaks names, an entry outside the reader table, or a
// tag key or parameter name that is not the harness's own.
func TestAWSStep0ReadSetHoldsNoIDs(t *testing.T) {
	data, err := os.ReadFile(awsStep0ReadSet)
	require.NoError(t, err)
	for what, re := range awsStep0Leaks {
		assert.Empty(t, re.FindAllString(string(data), -1), "the read set holds %s", what)
	}

	entries := loadAWSStep0ReadSet(t)
	require.NoError(t, harness.ValidateAWSReadSet(entries))
	require.Len(t, entries, step0AMIParameter+1)
	for _, i := range []int{step0VPCsByTag, step0GroupsByTag} {
		assert.JSONEq(t, `{"Filter.1.Name":"tag-key","Filter.1.Value.1":"`+harness.AWSRunIDTagKey+`"}`, string(entries[i].Params), "entry %d", i)
	}
	assert.JSONEq(t, `{"Name":"`+harness.AWSAL2023AMIParameter+`"}`, string(entries[step0AMIParameter].Params))
}

// TestAWSReadStep0AgainstFakeaws reads the committed step-0 read set
// through the walls against fakeaws, seeded with one VPC tagged
// AWSRunIDTagKey (with its subnet and SG) and one untagged VPC beside it.
// Each entry is answered at the pin, filters and zones included, and so
// is each follow-up but DescribeSecurityGroupRules (fakeawsPinRefusedRows).
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

	assert.ElementsMatch(t, fakeawsPinRefusedRows, slices.Collect(maps.Keys(doer.refused)), "the rows fakeaws refuses at the pin")
	assert.Empty(t, doer.unfiltered, "the rows whose filter fakeaws ignores at the pin")

	// Each entry is one page, so the replay's exchanges come first, in order.
	require.GreaterOrEqual(t, len(read.Exchanges), len(readSet))
	for i, entry := range readSet {
		assert.Equal(t, entry, read.Exchanges[i].AWSReadEntry, "entry %d has its exchange", i)
		assert.Equal(t, http.StatusOK, read.Exchanges[i].Status, "entry %d: %s", i, read.Exchanges[i].Body)
	}
	vpcIDs := regexp.MustCompile(`<vpcId>([^<]*)</vpcId>`)
	groupIDs := regexp.MustCompile(`<groupId>([^<]*)</groupId>`)
	assert.Equal(t, [][]string{{"<vpcId>" + tagged.vpc + "</vpcId>", tagged.vpc}}, vpcIDs.FindAllStringSubmatch(read.Exchanges[step0VPCsByTag].Body, -1), "the VPC by tag, and only it")
	assert.Equal(t, [][]string{{"<groupId>" + tagged.sg + "</groupId>", tagged.sg}}, groupIDs.FindAllStringSubmatch(read.Exchanges[step0GroupsByTag].Body, -1), "the SG by tag, and only it")
	assert.NotContains(t, read.Exchanges[step0AbsentGroup].Body, "<groupId>", "the absent name finds no group")
	assert.Contains(t, read.Exchanges[step0Zones].Body, "<zoneName>"+region+"a</zoneName>")
	assert.Contains(t, read.Exchanges[step0AMIParameter].Body, harness.AWSLayer2AMI)

	// The follow-ups the old pin refused (DescribeAvailabilityZones is the
	// entry above), and the seeded subnet, each answered for the tagged VPC.
	followUps := read.Exchanges[len(readSet):]
	inVPC := `"Filter.1.Value.1":"` + tagged.vpc + `"`
	for _, row := range []struct{ action, inBody string }{
		{"DescribeSecurityGroups", "<groupId>" + tagged.sg + "</groupId>"},
		{"DescribeNetworkInterfaces", "<networkInterfaceSet"},
		{"DescribeSubnets", "<subnetId>" + tagged.subnet + "</subnetId>"},
	} {
		assert.True(t, slices.ContainsFunc(followUps, func(e harness.AWSReadExchange) bool {
			return e.Action == row.action && e.Status == http.StatusOK && bytes.Contains(e.Params, []byte(inVPC)) && bytes.Contains([]byte(e.Body), []byte(row.inBody))
		}), "%s of %s", row.action, tagged.vpc)
	}
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
