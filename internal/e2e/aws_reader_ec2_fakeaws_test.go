package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// The EC2 follow-up rows fakeaws at FAKEAWS_SHA does not answer as EC2
// does, each as "Action" or "Action filter-name": fakeawsPinRefusedRows
// it refuses, fakeawsPinUnfilteredRows it answers with every VPC's items,
// ignoring the filter. At the pin, the SGs, ENIs and instances are
// answered from rows, so the rows they feed are sent: DescribeSecurityGroupRules
// (by the SGs' group-id) is sent and refused (not modelled; fakeaws-loader
// owns it), and DescribeAddresses (by the interfaces' network-interface-id)
// is sent and answered. DescribeKeyPairs by the instances' key-name is never
// sent, since fakeaws's DescribeInstances omits the key name.
var (
	fakeawsPinRefusedRows    = []string{"DescribeSecurityGroupRules group-id"}
	fakeawsPinUnfilteredRows = []string(nil)
)

var answerVPCIDs = regexp.MustCompile(`<vpcId>([^<]*)</vpcId>`)

// pinGapDoer sends each request to inner and records each EC2 row
// fakeaws does not answer as EC2 does. A refusal (any non-2xx answer)
// is answered with an empty 200 in its place, so the read goes on. An
// answer to a request filtered to one VPC that names another VPC is
// passed on as it is.
type pinGapDoer struct {
	inner               aws.HTTPClient
	refused, unfiltered map[string]bool
}

func (d *pinGapDoer) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	resp, err := d.inner.Do(req)
	if err != nil || !strings.Contains(req.URL.Path, "/ec2/") {
		return resp, err
	}
	form, err := neturl.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	row := strings.TrimSpace(form.Get("Action") + " " + form.Get("Filter.1.Name"))
	if resp.StatusCode >= http.StatusMultipleChoices {
		resp.Body.Close()
		d.refused[row] = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/xml"}},
			Body:       io.NopCloser(strings.NewReader("<" + form.Get("Action") + "Response/>")),
			Request:    req,
		}, nil
	}
	if name := form.Get("Filter.1.Name"); name == "vpc-id" || name == "attachment.vpc-id" {
		answer, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		resp.Body = io.NopCloser(bytes.NewReader(answer))
		for _, vpc := range answerVPCIDs.FindAllStringSubmatch(string(answer), -1) {
			if vpc[1] != form.Get("Filter.1.Value.1") {
				d.unfiltered[row] = true
			}
		}
	}
	return resp, nil
}

// TestAWSReadEC2AgainstFakeaws reads two of three seeded VPCs through
// the walls against fakeaws EC2, so the surroundings and follow-ups
// (aws_reader_ec2.go) are checked against fakeaws's own answers. The
// rows fakeaws refuses at the pin are exactly fakeawsPinRefusedRows. The
// required CI job runs it by name and fails if it does not report PASS.
func TestAWSReadEC2AgainstFakeaws(t *testing.T) {
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
	a := seedReaderVPC(ctx, t, ec2Client, "a")
	b := seedReaderVPC(ctx, t, ec2Client, "b")
	c := seedReaderVPC(ctx, t, ec2Client, "c")

	doer := &pinGapDoer{inner: loopback, refused: map[string]bool{}, unfiltered: map[string]bool{}}
	byID := fmt.Sprintf(`{"VpcId.1":%q,"VpcId.2":%q}`, a.vpc, b.vpc)
	readSet := []harness.AWSReadEntry{
		{Service: "ec2", Protocol: "query", Action: "DescribeVpcs", Params: json.RawMessage(byID)},
		{Service: "ec2", Protocol: "query", Action: "DescribeKeyPairs", Params: json.RawMessage(`{"KeyName.1":"` + a.keyName + `"}`)},
	}
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
	assert.ElementsMatch(t, fakeawsPinUnfilteredRows, slices.Collect(maps.Keys(doer.unfiltered)), "the rows whose filter fakeaws ignores at the pin")

	byRow := map[string][]harness.AWSReadExchange{}
	for _, exchange := range read.Exchanges {
		assert.Equal(t, http.StatusOK, exchange.Status, "%s %s", exchange.Action, exchange.Params)
		var params map[string]string
		require.NoError(t, json.Unmarshal(exchange.Params, &params))
		if !doer.unfiltered[strings.TrimSpace(exchange.Action+" "+params["Filter.1.Name"])] {
			assert.NotContains(t, string(exchange.Params)+exchange.Body, c.vpc, "the untouched VPC was read: %s", exchange.Action)
		}
		byRow[exchange.Action] = append(byRow[exchange.Action], exchange)
	}
	require.Len(t, byRow["DescribeVpcs"], 1)
	require.Len(t, byRow["DescribeKeyPairs"], 1, "the key-pair lookup, and no key-name read")
	assert.Contains(t, byRow["DescribeKeyPairs"][0].Body, "<keyName>"+a.keyName+"</keyName>")
	assert.NotContains(t, byRow["DescribeKeyPairs"][0].Body, "<keyName>"+b.keyName+"</keyName>")
	for _, s := range []seededReaderVPC{a, b} {
		assert.Contains(t, byRow["DescribeVpcs"][0].Body, "<vpcId>"+s.vpc+"</vpcId>")
		assert.True(t, anyExchange(byRow["DescribeSubnets"], s.vpc, s.subnet), "subnets of %s", s.vpc)
		assert.True(t, anyExchange(byRow["DescribeRouteTables"], s.vpc, s.routeTable), "route tables of %s", s.vpc)
		assert.True(t, anyExchange(byRow["DescribeInternetGateways"], s.vpc, s.gateway), "gateways of %s", s.vpc)
		assert.True(t, anyExchange(byRow["DescribeInstances"], s.vpc, s.instance), "instances of %s", s.vpc)
		assert.True(t, anyExchange(byRow["DescribeVpcAttribute"], `"enableDnsSupport"`, s.vpc), "enableDnsSupport of %s", s.vpc)
		assert.True(t, anyExchange(byRow["DescribeVpcAttribute"], `"enableDnsHostnames"`, s.vpc), "enableDnsHostnames of %s", s.vpc)
		for _, attribute := range []string{"instanceInitiatedShutdownBehavior", "disableApiStop", "disableApiTermination"} {
			assert.True(t, anyExchange(byRow["DescribeInstanceAttribute"], `"`+attribute+`"`, s.instance), "%s of %s", attribute, s.instance)
		}
		assert.True(t, anyExchange(byRow["DescribeInstanceAttribute"], `"userData"`, "<value>"+s.userData+"</value>"), "userData of %s, raw", s.instance)
	}
}

// seededReaderVPC is one VPC fakeaws holds, with its surroundings.
type seededReaderVPC struct {
	vpc, subnet, routeTable, gateway, instance, allocation, keyName, userData string
}

// seedReaderVPC creates a VPC with a subnet, SG, route table, attached
// gateway, key pair and an instance with user data and an address.
func seedReaderVPC(ctx context.Context, t *testing.T, c *ec2.Client, name string) seededReaderVPC {
	t.Helper()
	var s seededReaderVPC
	vpc, err := c.CreateVpc(ctx, &ec2.CreateVpcInput{CidrBlock: aws.String("10.0.0.0/16")})
	require.NoError(t, err)
	s.vpc = aws.ToString(vpc.Vpc.VpcId)
	subnet, err := c.CreateSubnet(ctx, &ec2.CreateSubnetInput{VpcId: vpc.Vpc.VpcId, CidrBlock: aws.String("10.0.1.0/24")})
	require.NoError(t, err)
	s.subnet = aws.ToString(subnet.Subnet.SubnetId)
	sg, err := c.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{VpcId: vpc.Vpc.VpcId, GroupName: aws.String("web"), Description: aws.String("web")})
	require.NoError(t, err)
	rt, err := c.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{VpcId: vpc.Vpc.VpcId})
	require.NoError(t, err)
	s.routeTable = aws.ToString(rt.RouteTable.RouteTableId)
	igw, err := c.CreateInternetGateway(ctx, &ec2.CreateInternetGatewayInput{})
	require.NoError(t, err)
	s.gateway = aws.ToString(igw.InternetGateway.InternetGatewayId)
	_, err = c.AttachInternetGateway(ctx, &ec2.AttachInternetGatewayInput{InternetGatewayId: igw.InternetGateway.InternetGatewayId, VpcId: vpc.Vpc.VpcId})
	require.NoError(t, err)
	s.keyName = "key-" + name
	_, err = c.ImportKeyPair(ctx, &ec2.ImportKeyPairInput{KeyName: aws.String(s.keyName), PublicKeyMaterial: []byte("ssh-rsa AAAAB3NzaC1yc2E")})
	require.NoError(t, err)
	s.userData = base64.StdEncoding.EncodeToString([]byte("#!/bin/sh\necho " + name + "\n"))
	run, err := c.RunInstances(ctx, &ec2.RunInstancesInput{
		ImageId: aws.String("ami-0abcd1234"), InstanceType: "t3.micro", MinCount: aws.Int32(1), MaxCount: aws.Int32(1),
		SubnetId: subnet.Subnet.SubnetId, SecurityGroupIds: []string{aws.ToString(sg.GroupId)},
		KeyName: aws.String(s.keyName), UserData: aws.String(s.userData),
	})
	require.NoError(t, err)
	s.instance = aws.ToString(run.Instances[0].InstanceId)
	eip, err := c.AllocateAddress(ctx, &ec2.AllocateAddressInput{})
	require.NoError(t, err)
	s.allocation = aws.ToString(eip.AllocationId)
	_, err = c.AssociateAddress(ctx, &ec2.AssociateAddressInput{AllocationId: eip.AllocationId, InstanceId: run.Instances[0].InstanceId})
	require.NoError(t, err)
	return s
}

// anyExchange reports whether one of exchanges names inParams in its
// params and inBody in its body.
func anyExchange(exchanges []harness.AWSReadExchange, inParams, inBody string) bool {
	for _, e := range exchanges {
		if strings.Contains(string(e.Params), inParams) && strings.Contains(e.Body, inBody) {
			return true
		}
	}
	return false
}
