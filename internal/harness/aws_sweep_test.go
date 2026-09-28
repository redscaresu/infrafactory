package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAWSScope answers the sweep's EC2 Describes and SSM
// DescribeParameters from scripted pages. polls[n][action][token] is
// action's page at token on its (n+1)th listing, a listing starting at
// each request with no token; the last poll repeats, and a page not
// scripted is empty. A denied action answers as AWS refuses it.
type fakeAWSScope struct {
	mu       sync.Mutex
	polls    []map[string]map[string]string
	denied   map[string]bool
	listings map[string]int
	requests []fakeAWSRequest
}

type fakeAWSRequest struct {
	action, body string
}

func newFakeAWSScope(polls ...map[string]map[string]string) *fakeAWSScope {
	return &fakeAWSScope{polls: polls, denied: map[string]bool{}, listings: map[string]int{}}
}

func (f *fakeAWSScope) Do(req *http.Request) (*http.Response, error) {
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var action, token string
	target := req.Header.Get("X-Amz-Target")
	if target != "" {
		action = strings.TrimPrefix(target, "AmazonSSM.")
		var in struct{ NextToken string }
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, err
		}
		token = in.NextToken
	} else {
		form, err := url.ParseQuery(string(payload))
		if err != nil {
			return nil, err
		}
		action, token = form.Get("Action"), form.Get("NextToken")
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, fakeAWSRequest{action: action, body: string(payload)})
	if token == "" {
		f.listings[action]++
	}
	status, body := http.StatusOK, scopePage(action, "")
	switch {
	case f.denied[action] && target != "":
		status, body = http.StatusBadRequest, `{"__type":"AccessDeniedException","message":"denied"}`
	case f.denied[action]:
		status, body = http.StatusForbidden, ec2UnauthorizedBody
	case len(f.polls) > 0:
		poll := f.polls[min(f.listings[action], len(f.polls))-1]
		if page, ok := poll[action][token]; ok {
			body = page
		}
	}
	contentType := "text/xml"
	if target != "" {
		contentType = "application/x-amz-json-1.1"
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

// sent returns the body of each request for action.
func (f *fakeAWSScope) sent(action string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var bodies []string
	for _, r := range f.requests {
		if r.action == action {
			bodies = append(bodies, r.body)
		}
	}
	return bodies
}

func (f *fakeAWSScope) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	actions := make([]string, len(f.requests))
	for i, r := range f.requests {
		actions[i] = r.action
	}
	return actions
}

// scopeSets is each EC2 Describe's result set element.
var scopeSets = map[string]string{
	"DescribeInstances":         "reservationSet",
	"DescribeVolumes":           "volumeSet",
	"DescribeNetworkInterfaces": "networkInterfaceSet",
	"DescribeAddresses":         "addressesSet",
	"DescribeSecurityGroups":    "securityGroupInfo",
	"DescribeVpcs":              "vpcSet",
	"DescribeSubnets":           "subnetSet",
	"DescribeInternetGateways":  "internetGatewaySet",
	"DescribeRouteTables":       "routeTableSet",
	"DescribeNatGateways":       "natGatewaySet",
	"DescribeKeyPairs":          "keySet",
	"DescribeImages":            "imagesSet",
	"DescribeSnapshots":         "snapshotSet",
	"DescribeLaunchTemplates":   "launchTemplates",
}

// scopePage is action's answer holding items, with next as its
// NextToken when non-empty. SSM items are JSON objects; EC2 items are
// <item> elements.
func scopePage(action, next string, items ...string) string {
	if action == "DescribeParameters" {
		page := `{"Parameters":[` + strings.Join(items, ",") + `]`
		if next != "" {
			page += `,"NextToken":"` + next + `"`
		}
		return page + `}`
	}
	set := scopeSets[action]
	page := fmt.Sprintf(`<%sResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><%s>%s</%s>`, action, set, strings.Join(items, ""), set)
	if next != "" {
		page += `<nextToken>` + next + `</nextToken>`
	}
	return page + `</` + action + `Response>`
}

// onePoll is a poll where each action's items fill its one page.
func onePoll(items map[string][]string) map[string]map[string]string {
	poll := map[string]map[string]string{}
	for action, its := range items {
		poll[action] = map[string]string{"": scopePage(action, "", its...)}
	}
	return poll
}

func instanceItem(id, state, tags string) string {
	return fmt.Sprintf(`<item><instancesSet><item><instanceId>%s</instanceId><instanceState><name>%s</name></instanceState>%s</item></instancesSet></item>`, id, state, tags)
}

func parameterItem(name string) string {
	return fmt.Sprintf(`{"Name":%q,"Type":"String"}`, name)
}

// sleeps records each settle wait, and never waits.
type sleeps struct {
	mu        sync.Mutex
	durations []time.Duration
}

func noSleep(context.Context, time.Duration) error { return nil }

func (s *sleeps) record(_ context.Context, d time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.durations = append(s.durations, d)
	return nil
}

func sweepFake(t *testing.T, fake *fakeAWSScope, sleep func(context.Context, time.Duration) error) ([]AWSStray, error) {
	t.Helper()
	return SweepAWSScope(context.Background(), scopeTestEnv(t), AWSDoers{EC2: fake, SSM: fake},
		AWSEndpoints{EC2: fakeDoerEndpoint, SSM: fakeDoerEndpoint}, sleep)
}

func TestSweepAWSScopeEmptyIsClean(t *testing.T) {
	t.Parallel()
	fake := newFakeAWSScope()
	var waits sleeps

	strays, err := sweepFake(t, fake, waits.record)

	require.NoError(t, err)
	assert.Empty(t, strays)
	assert.Empty(t, waits.durations)
	actions := fake.actions()
	assert.Len(t, actions, len(AWSSweepCollections), "one listing per collection")
	for _, action := range actions {
		assert.True(t, strings.HasPrefix(action, "Describe"), "the sweep only reads, and sent %s", action)
	}
}

// scopeRows plants one item per collection. paginated is false for the
// Describes whose API has no NextToken.
var scopeRows = []struct {
	collection, action, id, item string
	paginated                    bool
}{
	{"instances", "DescribeInstances", "i-0stray", instanceItem("i-0stray", "running", ""), true},
	{"volumes", "DescribeVolumes", "vol-0stray", `<item><volumeId>vol-0stray</volumeId><status>available</status></item>`, true},
	{"network interfaces", "DescribeNetworkInterfaces", "eni-0stray", `<item><networkInterfaceId>eni-0stray</networkInterfaceId><status>available</status></item>`, true},
	{"elastic ips", "DescribeAddresses", "eipalloc-0stray", `<item><allocationId>eipalloc-0stray</allocationId><publicIp>203.0.113.7</publicIp></item>`, false},
	{"security groups", "DescribeSecurityGroups", "sg-0stray", `<item><groupId>sg-0stray</groupId><groupName>web</groupName></item>`, true},
	{"vpcs", "DescribeVpcs", "vpc-0stray", `<item><vpcId>vpc-0stray</vpcId><state>available</state><isDefault>false</isDefault></item>`, true},
	{"subnets", "DescribeSubnets", "subnet-0stray", `<item><subnetId>subnet-0stray</subnetId><state>available</state></item>`, true},
	{"internet gateways", "DescribeInternetGateways", "igw-0stray", `<item><internetGatewayId>igw-0stray</internetGatewayId></item>`, true},
	{"route tables", "DescribeRouteTables", "rtb-0stray", `<item><routeTableId>rtb-0stray</routeTableId><associationSet><item><main>false</main><subnetId>subnet-0a</subnetId></item></associationSet></item>`, true},
	{"nat gateways", "DescribeNatGateways", "nat-0stray", `<item><natGatewayId>nat-0stray</natGatewayId><state>available</state></item>`, true},
	{"key pairs", "DescribeKeyPairs", "key-0stray", `<item><keyPairId>key-0stray</keyPairId><keyName>stray</keyName></item>`, false},
	{"images", "DescribeImages", "ami-0stray", `<item><imageId>ami-0stray</imageId><imageState>available</imageState></item>`, true},
	{"snapshots", "DescribeSnapshots", "snap-0stray", `<item><snapshotId>snap-0stray</snapshotId><status>completed</status></item>`, true},
	{"launch templates", "DescribeLaunchTemplates", "lt-0stray", `<item><launchTemplateId>lt-0stray</launchTemplateId></item>`, true},
	{"ssm parameters", "DescribeParameters", "/other/x", parameterItem("/other/x"), true},
	// Under the prefix, but neither the claim nor the stamp.
	{"ssm parameters", "DescribeParameters", AWSScopePrefix + "old-claim", parameterItem(AWSScopePrefix + "old-claim"), true},
}

func TestSweepAWSScopeNamesEachCollectionsStray(t *testing.T) {
	t.Parallel()

	for _, row := range scopeRows {
		t.Run(row.collection+" "+row.id, func(t *testing.T) {
			t.Parallel()

			t.Run("planted", func(t *testing.T) {
				t.Parallel()
				fake := newFakeAWSScope(onePoll(map[string][]string{row.action: {row.item}}))
				strays, err := sweepFake(t, fake, noSleep)
				require.Error(t, err)
				assert.Contains(t, err.Error(), row.collection+" "+row.id)
				require.Len(t, strays, 1)
				assert.Equal(t, row.collection, strays[0].Collection)
				assert.Equal(t, row.id, strays[0].ID)
			})

			if row.paginated {
				t.Run("on page 2", func(t *testing.T) {
					t.Parallel()
					fake := newFakeAWSScope(map[string]map[string]string{row.action: {
						"":   scopePage(row.action, "p2"),
						"p2": scopePage(row.action, "", row.item),
					}})
					_, err := sweepFake(t, fake, noSleep)
					require.Error(t, err)
					assert.Contains(t, err.Error(), row.collection+" "+row.id)
					assert.Len(t, fake.sent(row.action), 2)
				})

				t.Run("a token cycle", func(t *testing.T) {
					t.Parallel()
					fake := newFakeAWSScope(map[string]map[string]string{row.action: {
						"":  scopePage(row.action, "A"),
						"A": scopePage(row.action, "B"),
						"B": scopePage(row.action, "A"),
					}})
					strays, err := sweepFake(t, fake, noSleep)
					require.Error(t, err)
					assert.Contains(t, err.Error(), row.collection+": NextToken \"A\"")
					assert.Nil(t, strays)
				})
			} else {
				t.Run("sends no MaxResults", func(t *testing.T) {
					t.Parallel()
					fake := newFakeAWSScope()
					_, err := sweepFake(t, fake, noSleep)
					require.NoError(t, err)
					bodies := fake.sent(row.action)
					require.Len(t, bodies, 1)
					assert.NotContains(t, bodies[0], "MaxResults")
				})
			}

			t.Run("denied", func(t *testing.T) {
				t.Parallel()
				fake := newFakeAWSScope()
				fake.denied[row.action] = true
				strays, err := sweepFake(t, fake, noSleep)
				require.Error(t, err)
				assert.Contains(t, err.Error(), row.collection+": ")
				code := "UnauthorizedOperation"
				if row.action == "DescribeParameters" {
					code = "AccessDeniedException"
				}
				assert.Contains(t, err.Error(), code)
				assert.Nil(t, strays)
			})
		})
	}
}

// Each item here is not a leak. Removing its exclusion names it.
func TestSweepAWSScopeExclusions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, action, item string
	}{
		{"a terminated instance", "DescribeInstances", instanceItem("i-0gone", "terminated", "")},
		{"a VPC's default group", "DescribeSecurityGroups", `<item><groupId>sg-0def</groupId><groupName>default</groupName></item>`},
		{"a main route table", "DescribeRouteTables", `<item><routeTableId>rtb-0main</routeTableId><associationSet><item><main>true</main></item></associationSet></item>`},
		{"a deleted NAT gateway", "DescribeNatGateways", `<item><natGatewayId>nat-0gone</natGatewayId><state>deleted</state></item>`},
		{"the stamp", "DescribeParameters", parameterItem(AWSStampParameter)},
		{"the claim", "DescribeParameters", parameterItem(AWSClaimParameter)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeAWSScope(onePoll(map[string][]string{tc.action: {tc.item}}))
			strays, err := sweepFake(t, fake, noSleep)
			require.NoError(t, err)
			assert.Empty(t, strays)
		})
	}
}

func TestSweepAWSScopeNamesTheDefaultVPC(t *testing.T) {
	t.Parallel()
	fake := newFakeAWSScope(onePoll(map[string][]string{
		"DescribeVpcs": {`<item><vpcId>vpc-0def</vpcId><state>available</state><isDefault>true</isDefault></item>`},
	}))
	_, err := sweepFake(t, fake, noSleep)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vpcs vpc-0def")
}

func TestSweepAWSScopeSettles(t *testing.T) {
	t.Parallel()
	settling := map[string][]string{
		"DescribeInstances":         {instanceItem("i-0web", "shutting-down", "")},
		"DescribeNetworkInterfaces": {`<item><networkInterfaceId>eni-0web</networkInterfaceId><status>in-use</status></item>`},
		"DescribeVolumes":           {`<item><volumeId>vol-0root</volumeId><status>in-use</status></item>`},
	}

	t.Run("gone on the second poll", func(t *testing.T) {
		t.Parallel()
		fake := newFakeAWSScope(onePoll(settling), onePoll(nil))
		var waits sleeps

		strays, err := sweepFake(t, fake, waits.record)

		require.NoError(t, err)
		assert.Empty(t, strays)
		assert.Equal(t, []time.Duration{awsSweepSettleInterval}, waits.durations)
		for action := range scopeSets {
			assert.Len(t, fake.sent(action), 2, action)
		}
		assert.Len(t, fake.sent("DescribeParameters"), 2)
	})

	t.Run("still there at the bound", func(t *testing.T) {
		t.Parallel()
		fake := newFakeAWSScope(onePoll(settling))
		var waits sleeps

		strays, err := sweepFake(t, fake, waits.record)

		require.Error(t, err)
		for _, want := range []string{"instances i-0web (shutting-down)", "network interfaces eni-0web (in-use)", "volumes vol-0root (in-use)"} {
			assert.Contains(t, err.Error(), want)
		}
		assert.Len(t, strays, 3)
		assert.Len(t, waits.durations, AWSSweepMaxPolls-1)
		assert.Len(t, fake.sent("DescribeInstances"), AWSSweepMaxPolls)
	})

	t.Run("a cancelled wait fails", func(t *testing.T) {
		t.Parallel()
		fake := newFakeAWSScope(onePoll(settling))
		strays, err := sweepFake(t, fake, func(context.Context, time.Duration) error { return context.Canceled })
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, strays)
	})
}

// Each item is on its way out, so the sweep polls again.
func TestSweepAWSScopeSettlingStates(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, action, item string
	}{
		{"an instance shutting down", "DescribeInstances", instanceItem("i-0a", "shutting-down", "")},
		{"a NAT gateway deleting", "DescribeNatGateways", `<item><natGatewayId>nat-0a</natGatewayId><state>deleting</state></item>`},
		{"a volume deleting", "DescribeVolumes", `<item><volumeId>vol-0a</volumeId><status>deleting</status></item>`},
		{"a volume detaching", "DescribeVolumes", `<item><volumeId>vol-0a</volumeId><status>in-use</status><attachmentSet><item><status>detaching</status></item></attachmentSet></item>`},
		{"a network interface detaching", "DescribeNetworkInterfaces", `<item><networkInterfaceId>eni-0a</networkInterfaceId><status>detaching</status></item>`},
		{"an internet gateway detaching", "DescribeInternetGateways", `<item><internetGatewayId>igw-0a</internetGatewayId><attachmentSet><item><vpcId>vpc-0a</vpcId><state>detaching</state></item></attachmentSet></item>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeAWSScope(onePoll(map[string][]string{tc.action: {tc.item}}), onePoll(nil))
			var waits sleeps
			_, err := sweepFake(t, fake, waits.record)
			require.NoError(t, err)
			assert.Len(t, waits.durations, 1)
		})
	}
}

// Without Owner.1=self the listings hold every public image and
// snapshot; without IncludeDisabled a disabled image is hidden.
func TestSweepAWSScopeListsOnlyOwnImagesAndSnapshots(t *testing.T) {
	t.Parallel()
	fake := newFakeAWSScope()
	_, err := sweepFake(t, fake, noSleep)
	require.NoError(t, err)

	for action, want := range map[string]url.Values{
		"DescribeImages":    {"Owner.1": {"self"}, "IncludeDisabled": {"true"}},
		"DescribeSnapshots": {"Owner.1": {"self"}},
	} {
		bodies := fake.sent(action)
		require.Len(t, bodies, 1, action)
		form, err := url.ParseQuery(bodies[0])
		require.NoError(t, err)
		for key := range want {
			assert.Equal(t, want.Get(key), form.Get(key), "%s %s", action, key)
		}
	}
}

func TestSweepAWSScopeNamesTaggedAndUntaggedStrays(t *testing.T) {
	t.Parallel()
	tag := `<tagSet><item><key>` + AWSRunIDTagKey + `</key><value>run-7</value></item></tagSet>`
	fake := newFakeAWSScope(onePoll(map[string][]string{
		"DescribeInstances": {instanceItem("i-0tagged", "running", tag), instanceItem("i-0untagged", "running", "")},
	}))

	strays, err := sweepFake(t, fake, noSleep)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "instances i-0tagged (running, infrafactory-run-id=run-7)")
	assert.Contains(t, err.Error(), "instances i-0untagged (running)")
	assert.Equal(t, []AWSStray{
		{Collection: "instances", ID: "i-0tagged", State: "running", RunID: "run-7"},
		{Collection: "instances", ID: "i-0untagged", State: "running"},
	}, strays)
}
