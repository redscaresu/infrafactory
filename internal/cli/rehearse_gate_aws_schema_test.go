package cli_test

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/cli"
	"github.com/redscaresu/infrafactory/internal/e2e"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// TestRehearsalAWSProviderTableMatchesSchema: the rehearsal gate's
// provider "aws" table names exactly the attributes and nested blocks of
// provider "aws" in the harness.AWSProviderVersion schema, so a key the
// schema adds, or one the table names and the schema lacks, fails here
// (ADR-0042 § provider "aws").
func TestRehearsalAWSProviderTableMatchesSchema(t *testing.T) {
	e2e.SkipUnlessEnabled(t)
	t.Cleanup(e2e.CleanupAWSMirror)
	e2e.SealNetwork(t)

	raw, err := harness.ExtractProviderSchemaForCloud(context.Background(), sealedTofu(t), nil, "aws")
	require.NoError(t, err)
	var document struct {
		ProviderSchemas map[string]struct {
			Provider struct {
				Block struct {
					Attributes map[string]json.RawMessage `json:"attributes"`
					BlockTypes map[string]json.RawMessage `json:"block_types"`
				} `json:"block"`
			} `json:"provider"`
		} `json:"provider_schemas"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	block := document.ProviderSchemas["registry.opentofu.org/hashicorp/aws"].Provider.Block
	require.NotEmpty(t, block.Attributes, "no hashicorp/aws %s provider block in the extracted schema", harness.AWSProviderVersion)

	schema := slices.Concat(slices.Collect(maps.Keys(block.Attributes)), slices.Collect(maps.Keys(block.BlockTypes)))
	assert.ElementsMatch(t, schema, slices.Collect(maps.Keys(cli.RehearsalAWSProviderTable)),
		"the table and the hashicorp/aws %s provider schema disagree", harness.AWSProviderVersion)
}

// sealedTofu runs tofu through cli.RunTofu, under its deadline, with only
// the variables SealNetwork set and an empty HOME, so nothing in the shell
// reaches it.
func sealedTofu(t *testing.T) harness.CommandRunner {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	for _, key := range []string{"TF_CLI_CONFIG_FILE", "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY"} {
		env = append(env, key+"="+os.Getenv(key))
	}
	return harness.CommandRunnerFunc(func(_ context.Context, command harness.Command) (harness.CommandResult, error) {
		stdout, stderr, err := cli.RunTofu(command.Dir, env, command.Args...)
		return harness.CommandResult{Stdout: []byte(stdout), Stderr: []byte(stderr)}, err
	})
}
