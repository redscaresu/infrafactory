package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixtures are trimmed fakeaws /mock/state captures; testdata/topology/aws
// holds the HCL (main.tf) and the command (capture.sh) that produced them.
func deriveAWSFixture(t *testing.T, name string) (map[string]bool, map[string]string) {
	t.Helper()
	state, err := os.ReadFile(filepath.Join("testdata", "topology", "aws", name+".json"))
	require.NoError(t, err)
	return deriveAWSState(t, state)
}

func deriveAWSState(t *testing.T, state []byte) (map[string]bool, map[string]string) {
	t.Helper()
	require.Equal(t, "aws", detectCloud(state))

	body, diagnostics, err := DeriveTopology(state)
	require.NoError(t, err)
	var out struct {
		HTTPProbe    map[string]bool `json:"http_probe"`
		Connectivity map[string]bool `json:"connectivity"`
	}
	require.NoError(t, json.Unmarshal(body, &out))
	assert.Empty(t, out.Connectivity)
	return out.HTTPProbe, diagnostics
}

func TestDeriveTopologyAWS_PublicInstanceIsReachable(t *testing.T) {
	probe, diagnostics := deriveAWSFixture(t, "positive")
	assert.Equal(t, map[string]bool{"compute:80": true}, probe)
	assert.NotContains(t, diagnostics, "compute:80")
	assert.Equal(t, "tcp ingress on port 80", diagnostics["compute"])
}

func TestDeriveTopologyAWS_EachBrokenConditionIsUnreachable(t *testing.T) {
	cases := map[string]string{
		"unattached_group": "no security group attached to instance",
		"no_igw_route":     "has no 0.0.0.0/0 route to an internet gateway of vpc-",
		"no_association":   "has no route table association and vpc vpc-",
		"no_public_ip":     "has no public IPv4 address",
		"private_source":   "admits tcp 80 only from 10.0.0.0/16, not 0.0.0.0/0",
		"ipv6_only":        "admits tcp 80 only from ::/0: IPv6-only ingress",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			probe, diagnostics := deriveAWSFixture(t, name)
			assert.Equal(t, map[string]bool{"compute:80": false}, probe)
			assert.Contains(t, diagnostics["compute:80"], want)
		})
	}
}

// IDs in the no_association capture; captureRT routes 0.0.0.0/0 to the
// capture's internet gateway.
const (
	captureVPC    = "vpc-b71d3b208fbb518c"
	captureSubnet = "subnet-523903a5039f138a"
	captureRT     = "rtb-49a2fa0d0dc9bb9f"
)

// awsAssoc is a route table association as /mock/state exports it: a
// main one has no subnet.
func awsAssoc(table, subnet, state string) map[string]any {
	return map[string]any{"id": "rtbassoc-" + table, "route_table_id": table, "subnet_id": subnet, "gateway_id": "", "main": subnet == "", "state": state}
}

// awsFirst returns the first object of the ec2 list named key.
func awsFirst(t *testing.T, ec2 map[string]any, key string) map[string]any {
	t.Helper()
	list, ok := ec2[key].([]any)
	require.True(t, ok, "ec2.%s is not a list", key)
	require.NotEmpty(t, list, "ec2.%s is empty", key)
	first, ok := list[0].(map[string]any)
	require.True(t, ok, "ec2.%s[0] is not an object", key)
	return first
}

// awsCaptureWith returns the no_association capture with edit applied to
// its ec2 object.
func awsCaptureWith(t *testing.T, edit func(t *testing.T, ec2 map[string]any)) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "topology", "aws", "no_association.json"))
	require.NoError(t, err)
	var state map[string]any
	require.NoError(t, json.Unmarshal(raw, &state))
	ec2, ok := state["ec2"].(map[string]any)
	require.True(t, ok, "fixture ec2 is not an object")
	edit(t, ec2)
	body, err := json.Marshal(state)
	require.NoError(t, err)
	return body
}

// A subnet with no live explicit association uses its VPC's main route
// table. No capture holds a main association (fakeaws CreateVpc makes
// none; only /mock/load does), so each case adds associations to the
// no_association capture in the shape /mock/state exports them.
func TestDeriveTopologyAWS_UnassociatedSubnetUsesMainRouteTable(t *testing.T) {
	assocs := func(as ...map[string]any) func(*testing.T, map[string]any) {
		return func(t *testing.T, ec2 map[string]any) {
			tables, ok := ec2["route_tables"].([]any)
			require.True(t, ok, "ec2.route_tables is not a list")
			ec2["route_tables"] = append(tables,
				map[string]any{"id": "rtb-0private", "vpc_id": captureVPC},
				map[string]any{"id": "rtb-0other", "vpc_id": "vpc-other"})
			list := make([]any, len(as))
			for i, a := range as {
				list[i] = a
			}
			ec2["route_table_associations"] = list
		}
	}
	set := func(key, field, value string) func(*testing.T, map[string]any) {
		return func(t *testing.T, ec2 map[string]any) {
			assocs(awsAssoc(captureRT, "", ""))(t, ec2)
			awsFirst(t, ec2, key)[field] = value
		}
	}
	cases := map[string]struct {
		edit func(*testing.T, map[string]any)
		want string // "" when public
	}{
		"main table routes to the igw":            {assocs(awsAssoc(captureRT, "", "")), ""},
		"explicit private table beats the main":   {assocs(awsAssoc(captureRT, "", ""), awsAssoc("rtb-0private", captureSubnet, "associated")), "route table rtb-0private of subnet " + captureSubnet + " has no 0.0.0.0/0 route"},
		"no recorded state counts as associated":  {assocs(awsAssoc(captureRT, "", ""), awsAssoc("rtb-0private", captureSubnet, "")), "route table rtb-0private of subnet"},
		"disassociated falls back to the main":    {assocs(awsAssoc(captureRT, "", ""), awsAssoc("rtb-0private", captureSubnet, "disassociated")), ""},
		"another subnet's association isn't main": {assocs(awsAssoc(captureRT, "subnet-other", "associated")), "no main route table"},
		"main table of another vpc":               {assocs(awsAssoc("rtb-0other", "", "")), "no main route table"},
		"no main table":                           {func(*testing.T, map[string]any) {}, "has no route table association and vpc " + captureVPC + " has no main route table"},
		"main table with no igw route":            {assocs(awsAssoc("rtb-0private", "", "")), "main route table rtb-0private of vpc " + captureVPC + " (subnet " + captureSubnet + " has no association) has no 0.0.0.0/0 route"},
		"unknown subnet":                          {set("instances", "subnet_id", "subnet-gone"), "subnet subnet-gone is not in the mock state"},
		"instance with no subnet":                 {set("instances", "subnet_id", ""), "instance i-ffa00b3537f8ccb4 has no subnet"},
		"subnet with no vpc":                      {set("subnets", "vpc_id", ""), "subnet " + captureSubnet + " has no vpc"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			probe, diagnostics := deriveAWSState(t, awsCaptureWith(t, c.edit))
			assert.Equal(t, map[string]bool{"compute:80": c.want == ""}, probe)
			if c.want == "" {
				assert.NotContains(t, diagnostics, "compute:80")
				return
			}
			assert.Contains(t, diagnostics["compute:80"], c.want)
		})
	}
}

func TestDeriveTopologyAWS_UnderivedRulesEmitNoKey(t *testing.T) {
	cases := map[string]string{
		"port_range":     "tcp port range 80-90 on sg-",
		"all_protocol":   "protocol -1 (all traffic) rule on sg-",
		"no_public_rule": "no single-port tcp ingress rule with a cidr source",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			probe, diagnostics := deriveAWSFixture(t, name)
			assert.Empty(t, probe)
			assert.Contains(t, diagnostics["compute"], want)
		})
	}
}

// A probe on a port with no key reports the fallback, not a bare false.
func TestDeriveTopologyAWS_EvaluateUsesFallback(t *testing.T) {
	state, err := os.ReadFile(filepath.Join("testdata", "topology", "aws", "port_range.json"))
	require.NoError(t, err)
	failures, err := EvaluateTopology(state, []TopologyCheck{{Type: "http_probe", Target: "compute", Port: 80, Expect: "reachable"}})
	require.NoError(t, err)
	require.Len(t, failures, 1)
	assert.Contains(t, failures[0].Detail, "tcp port range 80-90")
}
