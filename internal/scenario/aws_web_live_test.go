package scenario

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// aws-web-live is step one of the AWS web stack HLD. Its visible criteria
// are pinned exactly: changing a name, a param or an expect fails here.
func TestAWSWebLiveScenarioIsPinned(t *testing.T) {
	repo := repoRootForAWSTest(t)
	sc, err := LoadWithSchema(filepath.Join(repo, "scenarios", "training", "aws-web-live.yaml"), filepath.Join(repo, "scenario.schema.json"))
	require.NoError(t, err)

	assert.Equal(t, "aws", sc.Cloud)
	require.NotNil(t, sc.Service)
	assert.Equal(t, "nginx", sc.Service.Image)
	assert.Equal(t, "1.27", sc.Service.Tag)
	assert.Equal(t, 80, sc.Service.Port)
	assert.NotEmpty(t, sc.Service.TTL)
	require.NotNil(t, sc.Resources.Compute)
	assert.Equal(t, "small", sc.Resources.Compute.Size)
	assert.Equal(t, 1, sc.Resources.Compute.Count)

	type crit struct {
		Type, Check, Region, Target string
		Port                        int
		Expect                      string
	}
	var got []crit
	for _, c := range sc.AcceptanceCriteria {
		g := crit{Type: c.Type, Check: c.Check, Target: c.Target, Expect: c.Expect}
		if c.Port != nil {
			g.Port = *c.Port
		}
		if r, ok := c.Params["region"].(string); ok {
			g.Region = r
		}
		got = append(got, g)
	}
	assert.Equal(t, []crit{
		{Type: "policy", Check: "region_restriction", Region: "us-east-1", Expect: "pass"},
		{Type: "policy", Check: "default_deny_ingress", Expect: "pass"},
		{Type: "http_probe", Target: "compute", Port: 80, Expect: "reachable"},
	}, got)
}

// The generator never sees a holdout check (ADR-0033): aws-web-live
// carries no connectivity criterion and none of its holdout's checks.
func TestAWSWebLiveHidesItsHoldout(t *testing.T) {
	repo := repoRootForAWSTest(t)
	schema := filepath.Join(repo, "scenario.schema.json")
	sc, err := LoadWithSchema(filepath.Join(repo, "scenarios", "training", "aws-web-live.yaml"), schema)
	require.NoError(t, err)
	holdout, err := LoadWithSchema(filepath.Join(repo, "scenarios", "holdout", "aws-web-live-unseen.yaml"), schema)
	require.NoError(t, err)

	for _, c := range sc.AcceptanceCriteria {
		assert.NotEqual(t, "connectivity", c.Type, "aws-web-live must not carry a connectivity criterion")
		for _, h := range holdout.AcceptanceCriteria {
			same := c.Type == h.Type && c.To == h.To && c.Expect == h.Expect && (c.Port == nil) == (h.Port == nil) && (c.Port == nil || *c.Port == *h.Port)
			assert.False(t, same, "holdout criterion %+v appears in aws-web-live", h)
		}
	}
}
