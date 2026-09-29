package harness

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAWSReapEC2 answers each EC2 action from script[action] in turn, the
// last answer repeating, each answer being a Describe's items. A Describe
// with no script answers an empty set, so the item is gone; any other
// action answers success, or fail[action]'s error code. Every request's
// form is recorded.
type fakeAWSReapEC2 struct {
	mu     sync.Mutex
	script map[string][]string
	fail   map[string]string
	calls  []url.Values
}

func newFakeAWSReapEC2(script map[string][]string) *fakeAWSReapEC2 {
	return &fakeAWSReapEC2{script: script, fail: map[string]string{}}
}

func (f *fakeAWSReapEC2) Do(req *http.Request) (*http.Response, error) {
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(payload))
	if err != nil {
		return nil, err
	}
	action := form.Get("Action")

	f.mu.Lock()
	defer f.mu.Unlock()
	answered := len(f.sentLocked(action))
	f.calls = append(f.calls, form)
	status := http.StatusOK
	body := `<` + action + `Response xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><return>true</return></` + action + `Response>`
	if code, failing := f.fail[action]; failing {
		status = http.StatusBadRequest
		body = `<Response><Errors><Error><Code>` + code + `</Code><Message>` + code + `</Message></Error></Errors><RequestID>0</RequestID></Response>`
	} else if answers := f.script[action]; len(answers) > 0 {
		body = scopePage(action, "", answers[min(answered, len(answers)-1)])
	} else if _, describe := scopeSets[action]; describe {
		body = scopePage(action, "")
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"text/xml"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func (f *fakeAWSReapEC2) sentLocked(action string) []url.Values {
	var forms []url.Values
	for _, form := range f.calls {
		if form.Get("Action") == action {
			forms = append(forms, form)
		}
	}
	return forms
}

func (f *fakeAWSReapEC2) sent(action string) []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sentLocked(action)
}

func (f *fakeAWSReapEC2) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	actions := make([]string, len(f.calls))
	for i, form := range f.calls {
		actions[i] = form.Get("Action")
	}
	return actions
}

// stsAnswer answers every STS request with its GetCallerIdentity body.
type stsAnswer string

func (body stsAnswer) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/xml"}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
		Request:    req,
	}, nil
}

// heldScope is a stamped scope testClaimHolder holds.
func heldScope() *fakeSSM {
	return newFakeSSM(map[string]string{AWSStampParameter: testAccount, AWSClaimParameter: testClaimHolder})
}

func reapFake(t *testing.T, ec2Doer *fakeAWSReapEC2, ssmDoer *fakeSSM, sts stsAnswer, waits *sleeps, strays []AWSStray) error {
	t.Helper()
	return ReapAWSScope(context.Background(), scopeTestEnv(t),
		AWSDoers{EC2: ec2Doer, SSM: ssmDoer, STS: sts},
		AWSEndpoints{EC2: fakeDoerEndpoint, SSM: fakeDoerEndpoint, STS: fakeDoerEndpoint},
		waits.record, testAccount, testAWSPrincipal, testClaimHolder, strays)
}

// reapRow plants one stray. calls is every EC2 action the reap sends
// for it, in order; params is "Action Param" → the value each of those
// calls sends.
type reapRow struct {
	name, collection, id string
	script               map[string][]string
	calls                []string
	params               map[string]string
	waits                int
}

var reapRows = []reapRow{
	{
		name: "an instance", collection: "instances", id: "i-0a",
		script: map[string][]string{"DescribeInstances": {instanceItem("i-0a", "shutting-down", ""), instanceItem("i-0a", "terminated", "")}},
		calls:  []string{"TerminateInstances", "DescribeInstances", "DescribeInstances"},
		params: map[string]string{"TerminateInstances InstanceId.1": "i-0a", "DescribeInstances InstanceId.1": "i-0a"},
		waits:  1,
	},
	{
		name: "a NAT gateway", collection: "nat gateways", id: "nat-0a",
		script: map[string][]string{"DescribeNatGateways": {
			`<item><natGatewayId>nat-0a</natGatewayId><state>deleting</state></item>`,
			`<item><natGatewayId>nat-0a</natGatewayId><state>deleted</state></item>`,
		}},
		calls:  []string{"DeleteNatGateway", "DescribeNatGateways", "DescribeNatGateways"},
		params: map[string]string{"DeleteNatGateway NatGatewayId": "nat-0a", "DescribeNatGateways NatGatewayId.1": "nat-0a"},
		waits:  1,
	},
	{
		name: "an associated Elastic IP", collection: "elastic ips", id: "eipalloc-0a",
		script: map[string][]string{"DescribeAddresses": {`<item><allocationId>eipalloc-0a</allocationId><associationId>eipassoc-0a</associationId><publicIp>203.0.113.7</publicIp></item>`}},
		calls:  []string{"DescribeAddresses", "DisassociateAddress", "ReleaseAddress"},
		params: map[string]string{"DescribeAddresses AllocationId.1": "eipalloc-0a", "DisassociateAddress AssociationId": "eipassoc-0a", "ReleaseAddress AllocationId": "eipalloc-0a"},
	},
	{
		name: "an unassociated Elastic IP", collection: "elastic ips", id: "eipalloc-0b",
		script: map[string][]string{"DescribeAddresses": {`<item><allocationId>eipalloc-0b</allocationId><publicIp>203.0.113.8</publicIp></item>`}},
		calls:  []string{"DescribeAddresses", "ReleaseAddress"},
		params: map[string]string{"ReleaseAddress AllocationId": "eipalloc-0b"},
	},
	{
		name: "a launch template", collection: "launch templates", id: "lt-0a",
		calls:  []string{"DeleteLaunchTemplate"},
		params: map[string]string{"DeleteLaunchTemplate LaunchTemplateId": "lt-0a"},
	},
	{
		name: "an image", collection: "images", id: "ami-0a",
		calls:  []string{"DeregisterImage"},
		params: map[string]string{"DeregisterImage ImageId": "ami-0a"},
	},
	{
		name: "a snapshot", collection: "snapshots", id: "snap-0a",
		calls:  []string{"DeleteSnapshot"},
		params: map[string]string{"DeleteSnapshot SnapshotId": "snap-0a"},
	},
	{
		name: "a volume", collection: "volumes", id: "vol-0a",
		script: map[string][]string{"DescribeVolumes": {
			`<item><volumeId>vol-0a</volumeId><status>in-use</status></item>`,
			`<item><volumeId>vol-0a</volumeId><status>available</status></item>`,
		}},
		calls:  []string{"DescribeVolumes", "DescribeVolumes", "DeleteVolume"},
		params: map[string]string{"DescribeVolumes VolumeId.1": "vol-0a", "DeleteVolume VolumeId": "vol-0a"},
		waits:  1,
	},
	{
		name: "a detached network interface", collection: "network interfaces", id: "eni-0a",
		script: map[string][]string{"DescribeNetworkInterfaces": {`<item><networkInterfaceId>eni-0a</networkInterfaceId><status>available</status></item>`}},
		calls:  []string{"DescribeNetworkInterfaces", "DescribeNetworkInterfaces", "DeleteNetworkInterface"},
		params: map[string]string{"DescribeNetworkInterfaces NetworkInterfaceId.1": "eni-0a", "DeleteNetworkInterface NetworkInterfaceId": "eni-0a"},
	},
	{
		// The detach is asynchronous: the delete waits for available.
		name: "an attached network interface", collection: "network interfaces", id: "eni-0b",
		script: map[string][]string{"DescribeNetworkInterfaces": {
			`<item><networkInterfaceId>eni-0b</networkInterfaceId><status>in-use</status><attachment><attachmentId>eni-attach-0b</attachmentId><status>attached</status></attachment></item>`,
			`<item><networkInterfaceId>eni-0b</networkInterfaceId><status>detaching</status><attachment><attachmentId>eni-attach-0b</attachmentId><status>detaching</status></attachment></item>`,
			`<item><networkInterfaceId>eni-0b</networkInterfaceId><status>available</status></item>`,
		}},
		calls:  []string{"DescribeNetworkInterfaces", "DetachNetworkInterface", "DescribeNetworkInterfaces", "DescribeNetworkInterfaces", "DeleteNetworkInterface"},
		params: map[string]string{"DetachNetworkInterface AttachmentId": "eni-attach-0b", "DeleteNetworkInterface NetworkInterfaceId": "eni-0b"},
		waits:  1,
	},
	{
		name: "a key pair", collection: "key pairs", id: "key-0a",
		calls:  []string{"DeleteKeyPair"},
		params: map[string]string{"DeleteKeyPair KeyPairId": "key-0a"},
	},
	{
		name: "a security group another group's rule names", collection: "security groups", id: "sg-0a",
		script: map[string][]string{"DescribeSecurityGroups": {`<item><groupId>sg-0a</groupId><groupName>web</groupName>` +
			`<ipPermissions><item><ipProtocol>tcp</ipProtocol><fromPort>443</fromPort><toPort>443</toPort><groups><item><groupId>sg-0b</groupId><userId>123456789012</userId></item></groups></item></ipPermissions>` +
			`<ipPermissionsEgress><item><ipProtocol>-1</ipProtocol><ipRanges><item><cidrIp>0.0.0.0/0</cidrIp></item></ipRanges></item></ipPermissionsEgress></item>`}},
		calls: []string{"DescribeSecurityGroups", "RevokeSecurityGroupIngress", "RevokeSecurityGroupEgress", "DeleteSecurityGroup"},
		params: map[string]string{
			"RevokeSecurityGroupIngress GroupId": "sg-0a", "RevokeSecurityGroupIngress IpPermissions.1.Groups.1.GroupId": "sg-0b",
			"RevokeSecurityGroupEgress GroupId": "sg-0a", "RevokeSecurityGroupEgress IpPermissions.1.IpRanges.1.CidrIp": "0.0.0.0/0",
			"DeleteSecurityGroup GroupId": "sg-0a",
		},
	},
	{
		name: "an associated route table", collection: "route tables", id: "rtb-0a",
		script: map[string][]string{"DescribeRouteTables": {`<item><routeTableId>rtb-0a</routeTableId><associationSet><item><routeTableAssociationId>rtbassoc-0a</routeTableAssociationId><main>false</main><subnetId>subnet-0a</subnetId></item></associationSet></item>`}},
		calls:  []string{"DescribeRouteTables", "DisassociateRouteTable", "DeleteRouteTable"},
		params: map[string]string{"DisassociateRouteTable AssociationId": "rtbassoc-0a", "DeleteRouteTable RouteTableId": "rtb-0a"},
	},
	{
		name: "a subnet", collection: "subnets", id: "subnet-0a",
		calls:  []string{"DeleteSubnet"},
		params: map[string]string{"DeleteSubnet SubnetId": "subnet-0a"},
	},
	{
		name: "an attached internet gateway", collection: "internet gateways", id: "igw-0a",
		script: map[string][]string{"DescribeInternetGateways": {`<item><internetGatewayId>igw-0a</internetGatewayId><attachmentSet><item><vpcId>vpc-0a</vpcId><state>available</state></item></attachmentSet></item>`}},
		calls:  []string{"DescribeInternetGateways", "DetachInternetGateway", "DeleteInternetGateway"},
		params: map[string]string{"DetachInternetGateway InternetGatewayId": "igw-0a", "DetachInternetGateway VpcId": "vpc-0a", "DeleteInternetGateway InternetGatewayId": "igw-0a"},
	},
	{
		name: "a VPC", collection: "vpcs", id: "vpc-0a",
		calls:  []string{"DeleteVpc"},
		params: map[string]string{"DeleteVpc VpcId": "vpc-0a"},
	},
}

func awsReapStep(t *testing.T, collection string) AWSReapStep {
	t.Helper()
	i := slices.IndexFunc(AWSReapSteps, func(s AWSReapStep) bool { return s.Collection == collection })
	require.GreaterOrEqual(t, i, 0, "no reap step for %s", collection)
	return AWSReapSteps[i]
}

func TestReapAWSScopeDeletesEachCollectionsStray(t *testing.T) {
	t.Parallel()

	for _, row := range reapRows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			fake, scope := newFakeAWSReapEC2(row.script), heldScope()
			var waits sleeps

			err := reapFake(t, fake, scope, getCallerIdentityXMLBody, &waits, []AWSStray{{Collection: row.collection, ID: row.id}})

			require.NoError(t, err)
			assert.Equal(t, row.calls, fake.actions())
			for key, want := range row.params {
				action, param, _ := strings.Cut(key, " ")
				forms := fake.sent(action)
				require.NotEmpty(t, forms, action)
				for _, form := range forms {
					assert.Equal(t, want, form.Get(param), key)
				}
			}
			step := awsReapStep(t, row.collection)
			for _, action := range fake.actions() {
				assert.Contains(t, step.EC2Actions, action, "%s sends an action its EC2Actions does not export", row.collection)
			}
			assert.Len(t, waits.durations, row.waits)
			assert.Zero(t, scope.writes(), "the reap never writes the claim")
		})
	}

	// With each row's calls asserted above, this makes EC2Actions exact:
	// an action exported but never sent would widen the IAM policy.
	t.Run("every exported action is sent by a row", func(t *testing.T) {
		t.Parallel()
		for _, step := range AWSReapSteps {
			for _, action := range step.EC2Actions {
				assert.True(t, slices.ContainsFunc(reapRows, func(row reapRow) bool {
					return row.collection == step.Collection && slices.Contains(row.calls, action)
				}), "%s exports %s, and no row sends it", step.Collection, action)
			}
		}
	})
}

// allReapStrays is one stray per reap row's collection, and the scripts
// that take each through its step.
func allReapStrays() ([]AWSStray, map[string][]string) {
	var strays []AWSStray
	script := map[string][]string{}
	for _, row := range reapRows {
		if slices.ContainsFunc(strays, func(s AWSStray) bool { return s.Collection == row.collection }) {
			continue
		}
		strays = append(strays, AWSStray{Collection: row.collection, ID: row.id})
		for action, answers := range row.script {
			script[action] = answers
		}
	}
	return strays, script
}

// Given the strays in reverse, the reap still runs its steps in order.
func TestReapAWSScopeRunsTheStepsInOrder(t *testing.T) {
	t.Parallel()
	strays, script := allReapStrays()
	slices.Reverse(strays)
	fake := newFakeAWSReapEC2(script)
	var waits sleeps

	require.NoError(t, reapFake(t, fake, heldScope(), getCallerIdentityXMLBody, &waits, strays))

	stepOf := map[string]int{}
	for i, step := range AWSReapSteps {
		for _, action := range step.EC2Actions {
			stepOf[action] = i
		}
	}
	var steps []int
	for _, action := range fake.actions() {
		steps = append(steps, stepOf[action])
	}
	assert.True(t, slices.IsSorted(steps), "calls out of step order: %v", fake.actions())
	assert.Len(t, slices.Compact(steps), len(AWSReapSteps), "every step sent something")
}

// Each gate failure returns before anything reaches EC2, and nothing
// writes to SSM.
func TestReapAWSScopeRefusesWithoutTheGate(t *testing.T) {
	t.Parallel()
	strays, script := allReapStrays()

	for _, tc := range []struct {
		name    string
		sts     stsAnswer
		params  map[string]string
		wantErr string
	}{
		{
			name: "STS names another account", sts: stsAnswer(strings.ReplaceAll(getCallerIdentityXMLBody, testAccount, "210987654321")),
			params: map[string]string{AWSStampParameter: testAccount, AWSClaimParameter: testClaimHolder}, wantErr: `account "210987654321"`,
		},
		{
			name: "the stamp is missing", sts: getCallerIdentityXMLBody,
			params: map[string]string{AWSClaimParameter: testClaimHolder}, wantErr: AWSStampParameter + " does not exist",
		},
		{
			name: "the stamp names another account", sts: getCallerIdentityXMLBody,
			params: map[string]string{AWSStampParameter: "210987654321", AWSClaimParameter: testClaimHolder}, wantErr: `holds "210987654321"`,
		},
		{
			name: "another holder holds the claim", sts: getCallerIdentityXMLBody,
			params: map[string]string{AWSStampParameter: testAccount, AWSClaimParameter: otherHolder}, wantErr: otherHolder,
		},
		{
			name: "no claim is held", sts: getCallerIdentityXMLBody,
			params: map[string]string{AWSStampParameter: testAccount}, wantErr: "no claim is held",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake, scope := newFakeAWSReapEC2(script), newFakeSSM(tc.params)
			var waits sleeps

			err := reapFake(t, fake, scope, tc.sts, &waits, strays)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, fake.actions(), "nothing reaches EC2")
			assert.Zero(t, scope.writes())
		})
	}
}

// A failed item is named and the rest still run; a NotFound is not a
// failure; a stray no step deletes is named, never sent; a settle that
// never ends is bounded.
func TestReapAWSScopeCarriesOnAndReportsPerItem(t *testing.T) {
	t.Parallel()
	fake := newFakeAWSReapEC2(map[string][]string{"DescribeInstances": {instanceItem("i-0a", "shutting-down", "")}})
	fake.fail["DeleteKeyPair"] = "UnauthorizedOperation"
	fake.fail["DeleteSubnet"] = "InvalidSubnetID.NotFound"
	scope := heldScope()
	var waits sleeps

	err := reapFake(t, fake, scope, getCallerIdentityXMLBody, &waits, []AWSStray{
		{Collection: "instances", ID: "i-0a"},
		{Collection: "key pairs", ID: "key-0a"},
		{Collection: "subnets", ID: "subnet-0a"},
		{Collection: "ssm parameters", ID: "/other/x"},
		{Collection: "vpcs", ID: "vpc-0a"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "instances i-0a: not terminated after 60 polls")
	assert.Len(t, waits.durations, AWSSweepMaxPolls-1)
	assert.Contains(t, err.Error(), "key pairs key-0a: ec2:DeleteKeyPair failed")
	assert.Contains(t, err.Error(), "UnauthorizedOperation")
	assert.NotContains(t, err.Error(), "subnet-0a")
	assert.Contains(t, err.Error(), "ssm parameters /other/x: reap does not delete this")
	assert.Len(t, fake.sent("DeleteVpc"), 1, "the VPC is still deleted")
	assert.Zero(t, scope.writes(), "no SSM parameter is deleted")
}

// awsReapPrecedence pairs collections whose reap must come first with
// those it must precede in real AWS. "internet gateways" is the IGW
// detach, which fails while the VPC has mapped public addresses.
var awsReapPrecedence = [][2]string{
	{"instances", "volumes"}, {"instances", "network interfaces"}, {"instances", "security groups"}, {"instances", "subnets"}, {"instances", "internet gateways"},
	{"nat gateways", "elastic ips"}, {"nat gateways", "subnets"}, {"nat gateways", "internet gateways"},
	{"elastic ips", "internet gateways"},
	{"images", "snapshots"},
	{"network interfaces", "security groups"}, {"network interfaces", "subnets"},
	{"security groups", "vpcs"}, {"subnets", "vpcs"}, {"route tables", "vpcs"}, {"internet gateways", "vpcs"},
}

func awsReapOrderViolations(order []string) []string {
	var violations []string
	for _, pair := range awsReapPrecedence {
		before, after := slices.Index(order, pair[0]), slices.Index(order, pair[1])
		if before < 0 || after < 0 || before > after {
			violations = append(violations, pair[0]+" must come before "+pair[1])
		}
	}
	return violations
}

func TestEveryAWSSweptCollectionHasAReapDelete(t *testing.T) {
	t.Parallel()
	var order []string
	for _, step := range AWSReapSteps {
		order = append(order, step.Collection)
		assert.NotEmpty(t, step.EC2Actions, step.Collection)
	}
	var swept, unreaped []string
	for _, row := range AWSSweepCollections {
		swept = append(swept, row.Name)
		if !slices.Contains(order, row.Name) {
			unreaped = append(unreaped, row.Name)
		}
	}
	assert.Equal(t, []string{"ssm parameters"}, unreaped, "every swept collection but the SSM row has a reap step")
	for _, collection := range order {
		assert.Contains(t, swept, collection, "a reap step for a collection the sweep does not list")
	}
	assert.Len(t, slices.Compact(slices.Sorted(slices.Values(order))), len(order), "one step per collection")

	assert.Empty(t, awsReapOrderViolations(order))
	for _, pair := range awsReapPrecedence {
		swapped := slices.Clone(order)
		i, j := slices.Index(swapped, pair[0]), slices.Index(swapped, pair[1])
		swapped[i], swapped[j] = swapped[j], swapped[i]
		assert.NotEmpty(t, awsReapOrderViolations(swapped), "swapping %s and %s must fail", pair[0], pair[1])
	}
}
