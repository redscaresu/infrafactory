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

// A subnet with no explicit association uses its VPC's main route table.
// No capture holds one (fakeaws CreateVpc makes none; only /mock/load
// does), so the main association is added to the no_association capture
// in the shape /mock/state exports it: main true, subnet_id "".
func TestDeriveTopologyAWS_UnassociatedSubnetUsesMainRouteTable(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "topology", "aws", "no_association.json"))
	require.NoError(t, err)
	withAssoc := func(vpcID, subnetID string, main bool) []byte {
		var state map[string]any
		require.NoError(t, json.Unmarshal(raw, &state))
		ec2 := state["ec2"].(map[string]any)
		rt := ec2["route_tables"].([]any)[0].(map[string]any)
		rt["vpc_id"] = vpcID
		ec2["route_table_associations"] = []any{map[string]any{
			"id": "rtbassoc-0main", "route_table_id": rt["id"], "subnet_id": subnetID, "gateway_id": "", "main": main, "state": "",
		}}
		body, err := json.Marshal(state)
		require.NoError(t, err)
		return body
	}

	const vpc = "vpc-b71d3b208fbb518c"
	probe, diagnostics := deriveAWSState(t, withAssoc(vpc, "", true))
	assert.Equal(t, map[string]bool{"compute:80": true}, probe)
	assert.NotContains(t, diagnostics, "compute:80")

	probe, diagnostics = deriveAWSState(t, withAssoc(vpc, "subnet-other", false))
	assert.Equal(t, map[string]bool{"compute:80": false}, probe, "another subnet's association is not the main table")
	assert.Contains(t, diagnostics["compute:80"], "no main route table")

	probe, diagnostics = deriveAWSState(t, withAssoc("vpc-other", "", true))
	assert.Equal(t, map[string]bool{"compute:80": false}, probe, "another VPC's main table does not route this subnet")
	assert.Contains(t, diagnostics["compute:80"], "no main route table")

	probe, diagnostics = deriveAWSState(t, raw)
	assert.Equal(t, map[string]bool{"compute:80": false}, probe, "no main table: unrouted")
	assert.Contains(t, diagnostics["compute:80"], "no main route table")
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
