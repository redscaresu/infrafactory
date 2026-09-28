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
		"no_association":   "has no route table association",
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
