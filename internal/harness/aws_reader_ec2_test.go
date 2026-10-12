package harness

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ec2RecordingFake answers each EC2 request with answer(form), an empty
// 200 for that action when answer gives none, and records every form.
type ec2RecordingFake struct {
	answer func(form url.Values) (awsCannedAnswer, bool)
	forms  []url.Values
}

func (f *ec2RecordingFake) Do(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	f.forms = append(f.forms, form)
	answer, ok := f.answer(form)
	if !ok {
		action := form.Get("Action")
		answer = awsCannedAnswer{body: "<" + action + "Response/>"}
	}
	return &http.Response{
		StatusCode: max(answer.status, http.StatusOK),
		Header:     http.Header{"Content-Type": {"text/xml"}},
		Body:       io.NopCloser(strings.NewReader(answer.body)),
		Request:    req,
	}, nil
}

// readEC2Fake reads readSet with fake answering EC2.
func readEC2Fake(t *testing.T, fake *ec2RecordingFake, readSet ...AWSReadEntry) (AWSRecordedRead, error) {
	t.Helper()
	doer := newAWSReadTestDoers().doer()
	doer[awsReadEC2Host] = fake
	return ReadAWSAccount(context.Background(), awsReadTestInput(doer, readSet...))
}

// ec2Form is the form a follow-up sends: Action, Version and params.
func ec2Form(action string, params map[string]string) url.Values {
	form := url.Values{"Action": {action}, "Version": {"2016-11-15"}}
	for k, v := range params {
		form.Set(k, v)
	}
	return form
}

func vpcFilter(name, value string) map[string]string {
	return map[string]string{"Filter.1.Name": name, "Filter.1.Value.1": value}
}

// ec2SurroundingsForms is every form readEC2VPC sends for a VPC holding
// one SG, interface and instance, each named after vpc.
func ec2SurroundingsForms(vpc string) []url.Values {
	forms := []url.Values{
		ec2Form("DescribeSubnets", vpcFilter("vpc-id", vpc)),
		ec2Form("DescribeSecurityGroups", vpcFilter("vpc-id", vpc)),
		ec2Form("DescribeSecurityGroupRules", vpcFilter("group-id", "sg-"+vpc)),
		ec2Form("DescribeRouteTables", vpcFilter("vpc-id", vpc)),
		ec2Form("DescribeInternetGateways", vpcFilter("attachment.vpc-id", vpc)),
		ec2Form("DescribeNetworkInterfaces", vpcFilter("vpc-id", vpc)),
		ec2Form("DescribeAddresses", vpcFilter("network-interface-id", "eni-"+vpc)),
		ec2Form("DescribeInstances", vpcFilter("vpc-id", vpc)),
		ec2Form("DescribeKeyPairs", vpcFilter("key-name", "key-"+vpc)),
		ec2Form("DescribeVolumes", vpcFilter("attachment.instance-id", "i-"+vpc)),
		ec2Form("DescribeVpcAttribute", map[string]string{"VpcId": vpc, "Attribute": "enableDnsSupport"}),
		ec2Form("DescribeVpcAttribute", map[string]string{"VpcId": vpc, "Attribute": "enableDnsHostnames"}),
	}
	for _, attribute := range []string{"instanceInitiatedShutdownBehavior", "disableApiStop", "disableApiTermination", "userData"} {
		forms = append(forms, ec2Form("DescribeInstanceAttribute", map[string]string{"InstanceId": "i-" + vpc, "Attribute": attribute}))
	}
	return forms
}

// ec2VPCAnswers answers each VPC-filtered list with one item named after
// the filter's VPC, and userData with that instance's user data.
func ec2VPCAnswers(form url.Values) (awsCannedAnswer, bool) {
	vpc := form.Get("Filter.1.Value.1")
	switch form.Get("Action") {
	case "DescribeSecurityGroups":
		// The rule refers to a peered VPC's SG, as EC2's UserIdGroupPair
		// does: neither that group nor vpc-c is this VPC's, and a
		// follow-up's answer touches no VPC.
		return awsCannedAnswer{body: `<DescribeSecurityGroupsResponse><securityGroupInfo><item><groupId>sg-` + vpc + `</groupId><vpcId>` + vpc +
			`</vpcId><ipPermissions><item><groups><item><groupId>sg-elsewhere</groupId><vpcId>vpc-c</vpcId></item></groups></item></ipPermissions></item></securityGroupInfo></DescribeSecurityGroupsResponse>`}, true
	case "DescribeNetworkInterfaces":
		return awsCannedAnswer{body: `<DescribeNetworkInterfacesResponse><networkInterfaceSet><item><networkInterfaceId>eni-` + vpc + `</networkInterfaceId></item></networkInterfaceSet></DescribeNetworkInterfacesResponse>`}, true
	case "DescribeInstances":
		return awsCannedAnswer{body: ec2InstancesBody("", "i-"+vpc, "key-"+vpc)}, true
	case "DescribeInstanceAttribute":
		if form.Get("Attribute") == "userData" {
			return awsCannedAnswer{body: readUserDataBody(form.Get("InstanceId"))}, true
		}
	}
	return awsCannedAnswer{}, false
}

func ec2InstancesBody(nextToken, instance, keyName string) string {
	return `<DescribeInstancesResponse><reservationSet><item><instancesSet><item><instanceId>` + instance + `</instanceId><keyName>` + keyName +
		`</keyName></item></instancesSet></item></reservationSet><nextToken>` + nextToken + `</nextToken></DescribeInstancesResponse>`
}

func readUserDataBody(instance string) string {
	return `<DescribeInstanceAttributeResponse><instanceId>` + instance + `</instanceId><userData><value>IyEvYmluL3NoCmVjaG8gJ3NlY3JldD0nCg==</value></userData></DescribeInstanceAttributeResponse>`
}

func TestReadAWSAccountReadsEachTouchedVPCsSurroundings(t *testing.T) {
	t.Parallel()

	// vpc-c is in the account, and an SG rule names it, but no replayed
	// answer does.
	const touched = `<DescribeVpcsResponse><vpcSet><item><vpcId>vpc-a</vpcId></item><item><vpcId>vpc-b</vpcId></item></vpcSet></DescribeVpcsResponse>`
	fake := &ec2RecordingFake{answer: func(form url.Values) (awsCannedAnswer, bool) {
		if form.Get("Action") == "DescribeVpcs" {
			return awsCannedAnswer{body: touched}, true
		}
		return ec2VPCAnswers(form)
	}}
	byTag := `{"Filter.1.Name":"tag-key","Filter.1.Value.1":"web"}`

	read, err := readEC2Fake(t, fake, readEntry("ec2", "query", "DescribeVpcs", byTag))
	require.NoError(t, err)

	want := []url.Values{ec2Form("DescribeVpcs", vpcFilter("tag-key", "web"))}
	want = append(want, ec2SurroundingsForms("vpc-a")...)
	want = append(want, ec2SurroundingsForms("vpc-b")...)
	want = append(want, ec2Form("DescribeAvailabilityZones", nil))
	assert.Equal(t, want, fake.forms, "each touched VPC's surroundings, each filter verbatim, the AZs once")
	for _, form := range fake.forms {
		assert.NotContains(t, form.Encode(), "vpc-c", "the untouched VPC was read")
		assert.NotContains(t, form.Encode(), "sg-elsewhere", "another VPC's SG was read")
	}

	require.Len(t, read.Exchanges, len(want))
	for i, exchange := range read.Exchanges {
		assert.Equal(t, want[i].Get("Action"), exchange.Action, "exchange %d", i)
		assert.Equal(t, http.StatusOK, exchange.Status, "exchange %d", i)
	}
	for _, instance := range []string{"i-vpc-a", "i-vpc-b"} {
		i := slices.IndexFunc(read.Exchanges, func(e AWSReadExchange) bool {
			return e.Action == "DescribeInstanceAttribute" && strings.Contains(string(e.Params), `"userData"`) && strings.Contains(string(e.Params), instance)
		})
		require.GreaterOrEqual(t, i, 0, "no userData read for %s", instance)
		assert.Equal(t, readUserDataBody(instance), read.Exchanges[i].Body, "userData is recorded raw")
	}
	assert.Equal(t, 1+len(want), read.Calls, "identity and each EC2 request, once")
}

// A VPC's instances' volumes are read in one request, filtered to every
// instance; a VPC with no instance reads none, since an empty filter
// would read the whole region.
func TestReadAWSAccountReadsAVPCsVolumesByItsInstances(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		instances string
		want      []url.Values
	}{
		"two instances": {
			instances: "<item><instanceId>i-1</instanceId></item><item><instanceId>i-2</instanceId></item>",
			want: []url.Values{ec2Form("DescribeVolumes", map[string]string{
				"Filter.1.Name": "attachment.instance-id", "Filter.1.Value.1": "i-1", "Filter.1.Value.2": "i-2",
			})},
		},
		"no instance": {},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := &ec2RecordingFake{answer: func(form url.Values) (awsCannedAnswer, bool) {
				switch form.Get("Action") {
				case "DescribeVpcs":
					return awsCannedAnswer{body: `<DescribeVpcsResponse><vpcSet><item><vpcId>vpc-a</vpcId></item></vpcSet></DescribeVpcsResponse>`}, true
				case "DescribeInstances":
					return awsCannedAnswer{body: `<DescribeInstancesResponse><reservationSet><item><instancesSet>` + tc.instances +
						`</instancesSet></item></reservationSet></DescribeInstancesResponse>`}, true
				}
				return awsCannedAnswer{}, false
			}}

			_, err := readEC2Fake(t, fake, readEntry("ec2", "query", "DescribeVpcs", `{}`))
			require.NoError(t, err)

			var volumes []url.Values
			for _, form := range fake.forms {
				if form.Get("Action") == "DescribeVolumes" {
					volumes = append(volumes, form)
				}
			}
			assert.Equal(t, tc.want, volumes)
		})
	}
}

func TestReadAWSAccountReadsTheAZsOnlyOnceWhenTheReplayReadThem(t *testing.T) {
	t.Parallel()

	fake := &ec2RecordingFake{answer: func(form url.Values) (awsCannedAnswer, bool) {
		if form.Get("Action") == "DescribeVpcs" {
			return awsCannedAnswer{body: `<DescribeVpcsResponse><vpcSet><item><vpcId>vpc-a</vpcId></item></vpcSet></DescribeVpcsResponse>`}, true
		}
		return awsCannedAnswer{}, false
	}}

	_, err := readEC2Fake(t, fake, readEntry("ec2", "query", "DescribeAvailabilityZones", `{}`), readEntry("ec2", "query", "DescribeVpcs", `{}`))
	require.NoError(t, err)

	zones := 0
	for _, form := range fake.forms {
		if form.Get("Action") == "DescribeAvailabilityZones" {
			zones++
		}
	}
	assert.Equal(t, 1, zones)
}

func TestReadAWSAccountTouchesNoVPCAndReadsNothingMore(t *testing.T) {
	t.Parallel()

	fake := &ec2RecordingFake{answer: func(url.Values) (awsCannedAnswer, bool) { return awsCannedAnswer{}, false }}

	read, err := readEC2Fake(t, fake, readEntry("ec2", "query", "DescribeVpcs", `{}`))
	require.NoError(t, err)
	assert.Len(t, fake.forms, 1)
	assert.Len(t, read.Exchanges, 1)
}

func TestReadAWSAccountPagesAFollowUpToItsEnd(t *testing.T) {
	t.Parallel()

	sgByTag := readEntry("ec2", "query", "DescribeSecurityGroups", `{"Filter.1.Name":"tag-key","Filter.1.Value.1":"web"}`)
	pagedInstances := func(secondToken string) func(url.Values) (awsCannedAnswer, bool) {
		return func(form url.Values) (awsCannedAnswer, bool) {
			if form.Get("Action") != "DescribeInstances" {
				return ec2VPCAnswers(form)
			}
			if form.Get("NextToken") == "" {
				return awsCannedAnswer{body: ec2InstancesBody("page-2", "i-1", "")}, true
			}
			return awsCannedAnswer{body: ec2InstancesBody(secondToken, "i-2", "")}, true
		}
	}
	vpcAnswer := func(f func(url.Values) (awsCannedAnswer, bool)) func(url.Values) (awsCannedAnswer, bool) {
		return func(form url.Values) (awsCannedAnswer, bool) {
			if form.Get("Action") == "DescribeSecurityGroups" && form.Get("Filter.1.Name") == "tag-key" {
				return awsCannedAnswer{body: `<DescribeSecurityGroupsResponse><securityGroupInfo><item><groupId>sg-1</groupId><vpcId>vpc-a</vpcId></item></securityGroupInfo></DescribeSecurityGroupsResponse>`}, true
			}
			return f(form)
		}
	}

	t.Run("two pages are recorded whole", func(t *testing.T) {
		t.Parallel()
		fake := &ec2RecordingFake{answer: vpcAnswer(pagedInstances(""))}

		read, err := readEC2Fake(t, fake, sgByTag)
		require.NoError(t, err)

		var pages []AWSReadExchange
		for _, e := range read.Exchanges {
			if e.Action == "DescribeInstances" {
				pages = append(pages, e)
			}
		}
		require.Len(t, pages, 2)
		assert.JSONEq(t, `{"Filter.1.Name":"vpc-id","Filter.1.Value.1":"vpc-a"}`, string(pages[0].Params))
		assert.JSONEq(t, `{"Filter.1.Name":"vpc-id","Filter.1.Value.1":"vpc-a","NextToken":"page-2"}`, string(pages[1].Params))
		assert.Contains(t, pages[1].Body, "<instanceId>i-2</instanceId>")
		for _, instance := range []string{"i-1", "i-2"} {
			assert.Contains(t, fake.forms, ec2Form("DescribeInstanceAttribute", map[string]string{"InstanceId": instance, "Attribute": "userData"}),
				"the instance on each page gets its attributes")
		}
	})
	t.Run("a repeated nextToken fails, naming the call", func(t *testing.T) {
		t.Parallel()
		fake := &ec2RecordingFake{answer: vpcAnswer(pagedInstances("page-2"))}

		read, err := readEC2Fake(t, fake, sgByTag)

		require.Error(t, err)
		assert.Contains(t, err.Error(), `follow-up DescribeInstances {"Filter.1.Name":"vpc-id","Filter.1.Value.1":"vpc-a"}`)
		assert.Contains(t, err.Error(), `NextToken "page-2" came back a second time`)
		assert.Empty(t, read.Exchanges)
	})
}

func TestReadAWSAccountLooksUpAKeyPairByName(t *testing.T) {
	t.Parallel()

	const match = `<DescribeKeyPairsResponse><keySet><item><keyName>deploy</keyName><keyPairId>key-0123</keyPairId></item></keySet></DescribeKeyPairsResponse>`
	fake := &ec2RecordingFake{answer: func(form url.Values) (awsCannedAnswer, bool) {
		if form.Get("KeyName.1") == "deploy" {
			return awsCannedAnswer{body: match}, true
		}
		return awsCannedAnswer{}, false
	}}

	read, err := readEC2Fake(t, fake, readEntry("ec2", "query", "DescribeKeyPairs", `{"KeyName.1":"deploy"}`))
	require.NoError(t, err)

	require.Len(t, fake.forms, 1)
	assert.Equal(t, ec2Form("DescribeKeyPairs", map[string]string{"KeyName.1": "deploy"}), fake.forms[0])
	require.Len(t, read.Exchanges, 1)
	assert.Equal(t, match, read.Exchanges[0].Body)
}

// The fake holds an SG named web in vpc-a and another in vpc-b, and
// answers as EC2 does: GroupName.N finds only a default-VPC SG, so here
// none; group-name and vpc-id filters find the one in that VPC.
func TestReadAWSAccountLooksUpAnSGWithinAVPC(t *testing.T) {
	t.Parallel()

	sgs := map[string]string{"vpc-a": "sg-a", "vpc-b": "sg-b"}
	fake := &ec2RecordingFake{answer: func(form url.Values) (awsCannedAnswer, bool) {
		if form.Get("Action") != "DescribeSecurityGroups" || form.Get("Filter.1.Value.1") != "web" {
			return awsCannedAnswer{}, false
		}
		if form.Has("GroupName.1") {
			return awsCannedAnswer{status: http.StatusBadRequest, body: ec2ErrorBody("InvalidGroup.NotFound")}, true
		}
		items := ""
		for vpc, sg := range sgs {
			if form.Get("Filter.1.Name") == "group-name" && form.Get("Filter.2.Name") == "vpc-id" && form.Get("Filter.2.Value.1") == vpc {
				items += "<item><groupId>" + sg + "</groupId><groupName>web</groupName><vpcId>" + vpc + "</vpcId></item>"
			}
		}
		return awsCannedAnswer{body: "<DescribeSecurityGroupsResponse><securityGroupInfo>" + items + "</securityGroupInfo></DescribeSecurityGroupsResponse>"}, true
	}}
	lookup := `{"Filter.1.Name":"group-name","Filter.1.Value.1":"web","Filter.2.Name":"vpc-id","Filter.2.Value.1":"vpc-a"}`

	read, err := readEC2Fake(t, fake, readEntry("ec2", "query", "DescribeSecurityGroups", lookup))
	require.NoError(t, err)

	require.NotEmpty(t, fake.forms)
	assert.Equal(t, ec2Form("DescribeSecurityGroups", map[string]string{
		"Filter.1.Name": "group-name", "Filter.1.Value.1": "web", "Filter.2.Name": "vpc-id", "Filter.2.Value.1": "vpc-a",
	}), fake.forms[0], "the filters reach the wire verbatim")
	for _, form := range fake.forms {
		for key := range form {
			assert.False(t, strings.HasPrefix(key, "GroupName."), "a request sent %s", key)
		}
	}
	require.NotEmpty(t, read.Exchanges)
	assert.Equal(t, http.StatusOK, read.Exchanges[0].Status)
	assert.Contains(t, read.Exchanges[0].Body, "<groupId>sg-a</groupId>")
	assert.NotContains(t, read.Exchanges[0].Body, "sg-b", "the same-named SG in another VPC")
}

// A replayed answer's own resource names vpc-a; its ingress and egress
// rules name a peered group in vpc-c, which the read did not touch.
func TestReadAWSAccountTakesOnlyAListedResourcesOwnVPC(t *testing.T) {
	t.Parallel()

	const peer = `<groups><item><userId>111122223333</userId><groupId>sg-peer</groupId><vpcId>vpc-c</vpcId>` +
		`<vpcPeeringConnectionId>pcx-1</vpcPeeringConnectionId></item></groups>`
	const replayed = `<DescribeSecurityGroupsResponse><securityGroupInfo><item><groupId>sg-a</groupId><groupName>web</groupName><vpcId>vpc-a</vpcId>` +
		`<ipPermissions><item>` + peer + `</item></ipPermissions><ipPermissionsEgress><item>` + peer + `</item></ipPermissionsEgress>` +
		`</item></securityGroupInfo></DescribeSecurityGroupsResponse>`
	fake := &ec2RecordingFake{answer: func(form url.Values) (awsCannedAnswer, bool) {
		if form.Get("Action") == "DescribeSecurityGroups" && form.Get("Filter.1.Name") == "group-name" {
			return awsCannedAnswer{body: replayed}, true
		}
		return awsCannedAnswer{}, false
	}}

	_, err := readEC2Fake(t, fake, readEntry("ec2", "query", "DescribeSecurityGroups", `{"Filter.1.Name":"group-name","Filter.1.Value.1":"web"}`))
	require.NoError(t, err)

	assert.Contains(t, fake.forms, ec2Form("DescribeSubnets", vpcFilter("vpc-id", "vpc-a")), "the listed SG's own VPC is read")
	for _, form := range fake.forms[1:] {
		assert.NotContains(t, form.Encode(), "vpc-c", "the peered VPC was read: %s", form.Get("Action"))
	}
}

func TestAWSListedVPCIDs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, body string
		want       []string
	}{
		{"each listed VPC", `<DescribeVpcsResponse><vpcSet><item><vpcId>vpc-a</vpcId></item><item><vpcId>vpc-b</vpcId></item></vpcSet></DescribeVpcsResponse>`,
			[]string{"vpc-a", "vpc-b"}},
		{"an instance's own VPC, not its interface's", `<DescribeInstancesResponse><reservationSet><item><instancesSet><item><vpcId>vpc-a</vpcId>` +
			`<networkInterfaceSet><item><vpcId>vpc-c</vpcId></item></networkInterfaceSet></item></instancesSet></item></reservationSet></DescribeInstancesResponse>`,
			[]string{"vpc-a"}},
		{"not a gateway's attachment", `<DescribeInternetGatewaysResponse><internetGatewaySet><item><attachmentSet><item><vpcId>vpc-c</vpcId></item></attachmentSet></item></internetGatewaySet></DescribeInternetGatewaysResponse>`,
			nil},
		{"not a VPC attribute answer's root field", `<DescribeVpcAttributeResponse><vpcId>vpc-c</vpcId></DescribeVpcAttributeResponse>`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := awsListedVPCIDs([]byte(tc.body))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
