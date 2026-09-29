package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// awsAllowlistCollection is the swept collection each allowlisted aws_
// type's resource lands in. A route and a route table association live
// in their route table.
var awsAllowlistCollection = map[string]string{
	"aws_vpc":                     "vpcs",
	"aws_subnet":                  "subnets",
	"aws_internet_gateway":        "internet gateways",
	"aws_route_table":             "route tables",
	"aws_route":                   "route tables",
	"aws_route_table_association": "route tables",
	"aws_security_group":          "security groups",
	"aws_instance":                "instances",
	"aws_eip":                     "elastic ips",
}

// awsAllowlistGaps names each aws_ entry whose collection the sweep does
// not list or the reap does not delete. An allowlist with no aws_ entry
// is a gap too, so this cannot pass before the entries exist.
func awsAllowlistGaps(allowlist []string) []string {
	swept := map[string]bool{}
	for _, row := range harness.AWSSweepCollections {
		swept[row.Name] = true
	}
	reaped := map[string]bool{}
	for _, step := range harness.AWSReapSteps {
		reaped[step.Collection] = true
	}
	var gaps []string
	awsEntries := 0
	for _, entry := range allowlist {
		if !strings.HasPrefix(entry, "aws_") {
			continue
		}
		awsEntries++
		collection, mapped := awsAllowlistCollection[entry]
		switch {
		case !mapped:
			gaps = append(gaps, entry+" maps to no swept collection")
		case !swept[collection]:
			gaps = append(gaps, entry+" maps to "+collection+", which the sweep does not list")
		case !reaped[collection]:
			gaps = append(gaps, entry+" maps to "+collection+", which the reap does not delete")
		}
	}
	if awsEntries == 0 {
		gaps = append(gaps, "the allowlist has no aws_ entry")
	}
	return gaps
}

// Every AWS type Layer 3 may apply leaves something the sweep can find
// and the reap can delete.
func TestEveryAWSAllowlistedTypeIsSweptAndReaped(t *testing.T) {
	t.Parallel()
	checkedIn, err := config.Load(filepath.Join("..", "..", "infrafactory.yaml"))
	require.NoError(t, err)
	defaults := config.Default().Validation.Layers.SandboxDeploy.AllowResourceTypes

	assert.Empty(t, awsAllowlistGaps(defaults), "config.Default()")
	assert.Empty(t, awsAllowlistGaps(checkedIn.Validation.Layers.SandboxDeploy.AllowResourceTypes), "infrafactory.yaml")

	assert.NotEmpty(t, awsAllowlistGaps(append(defaults[:len(defaults):len(defaults)], "aws_not_a_mapped_type")), "an unmapped aws_ type must fail")
	assert.NotEmpty(t, awsAllowlistGaps([]string{"scaleway_instance_server"}), "an allowlist with no aws_ entry must fail")
	assert.NotEmpty(t, awsAllowlistGaps(nil), "an empty allowlist must fail")
}
