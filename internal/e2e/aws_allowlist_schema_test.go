package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// awsProviderSchemaKey is hashicorp/aws's key in `tofu providers schema
// -json`.
const awsProviderSchemaKey = "registry.opentofu.org/hashicorp/aws"

// awsResourceSchemas is the part of `tofu providers schema -json` this
// file reads: each resource type's top-level attributes.
type awsResourceSchemas map[string]struct {
	Block struct {
		Attributes map[string]json.RawMessage `json:"attributes"`
	} `json:"block"`
}

// unplaceableAWSTypes names every aws_ allowlist entry that
// harness.UnplacedAWSResources could never place: not a child, and
// carrying neither arn nor owner_id in schema. A type missing from schema
// carries neither, so a typo or a glob is named too.
func unplaceableAWSTypes(schema awsResourceSchemas, allowlist []string, children map[string][]string) []string {
	var unplaceable []string
	for _, resourceType := range allowlist {
		if !strings.HasPrefix(resourceType, "aws_") {
			continue
		}
		if _, child := children[resourceType]; child {
			continue
		}
		attributes := schema[resourceType].Block.Attributes
		_, arn := attributes["arn"]
		_, owner := attributes["owner_id"]
		if !arn && !owner {
			unplaceable = append(unplaceable, resourceType)
		}
	}
	return unplaceable
}

func TestUnplaceableAWSTypesNamesATypeWithNoAccount(t *testing.T) {
	schema := awsResourceSchemas{}
	for resourceType, attributes := range map[string][]string{
		"aws_with_arn":   {"id", "arn"},
		"aws_with_owner": {"id", "owner_id"},
		"aws_route":      {"id", "route_table_id"},
		"aws_no_account": {"id", "tags"},
	} {
		entry := schema[resourceType]
		entry.Block.Attributes = map[string]json.RawMessage{}
		for _, attribute := range attributes {
			entry.Block.Attributes[attribute] = json.RawMessage(`{}`)
		}
		schema[resourceType] = entry
	}

	got := unplaceableAWSTypes(schema,
		[]string{"scaleway_vpc", "aws_with_arn", "aws_with_owner", "aws_route", "aws_no_account", "aws_absent"},
		harness.AWSChildScopedTypes)

	assert.Equal(t, []string{"aws_no_account", "aws_absent"}, got)
}

// TestE2E_AWSAllowlistPlaceable reads the pinned hashicorp/aws schema and
// requires every aws_ entry of the default allowlist to be placeable by
// harness.UnplacedAWSResources. aws_eip passes by its arn, which the
// provider builds from its own account. The required CI job runs it by
// name and fails if it does not report PASS.
func TestE2E_AWSAllowlistPlaceable(t *testing.T) {
	SkipUnlessEnabled(t)
	_, err := exec.LookPath("tofu")
	require.NoError(t, err, "tofu binary required for e2e")
	SealNetwork(t)

	raw, err := harness.ExtractProviderSchemaForCloud(context.Background(), tofuRunner, nil, "aws")
	require.NoError(t, err)
	var document struct {
		ProviderSchemas map[string]struct {
			ResourceSchemas awsResourceSchemas `json:"resource_schemas"`
		} `json:"provider_schemas"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	schema := document.ProviderSchemas[awsProviderSchemaKey].ResourceSchemas
	require.NotEmpty(t, schema, "no %s resource schemas in the extracted schema", awsProviderSchemaKey)

	allowlist := config.Default().Validation.Layers.SandboxDeploy.AllowResourceTypes
	assert.Contains(t, allowlist, "aws_eip")
	assert.Contains(t, schema["aws_eip"].Block.Attributes, "arn")
	assert.Empty(t, unplaceableAWSTypes(schema, allowlist, harness.AWSChildScopedTypes))
}

// tofuRunner runs a harness.Command with this process's environment, so
// tofu sees SealNetwork's mirror and dead proxy.
var tofuRunner = harness.CommandRunnerFunc(func(ctx context.Context, command harness.Command) (harness.CommandResult, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Dir = command.Dir
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return harness.CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
})
