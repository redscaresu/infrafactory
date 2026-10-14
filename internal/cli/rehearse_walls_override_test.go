package cli

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

const (
	overrideTestFakeaws = "http://127.0.0.1:4566"
	overrideTestRegion  = "eu-west-1"
)

var (
	overrideTestKeys    = []string{"ec2", "s3", "sts", "elbv2"}
	overrideTestAliases = []string{"west", "east"}
)

// The exact bytes, for UPDATE_GOLDEN=1 to rewrite. What they must say is
// asserted outside the golden, which certifies whatever the code makes.
func TestRehearsalProviderOverrideGolden(t *testing.T) {
	t.Parallel()

	got := rehearsalProviderOverride(overrideTestFakeaws, overrideTestRegion, overrideTestKeys, overrideTestAliases)

	path := filepath.Join("testdata", "golden", "walls", "provider_override.tf")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.WriteFile(path, got, 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got), "golden mismatch for %s (set UPDATE_GOLDEN=1 to refresh)", path)
}

// Every block holds each line, compared with its whitespace collapsed:
// modelled keys at fakeaws with the region filled, the rest at the dead
// proxy.
func TestRehearsalProviderOverrideLiteralLines(t *testing.T) {
	t.Parallel()

	override := string(rehearsalProviderOverride(overrideTestFakeaws+"/", overrideTestRegion, overrideTestKeys, overrideTestAliases))

	var lines []string
	for _, line := range strings.Split(override, "\n") {
		lines = append(lines, strings.Join(strings.Fields(line), " "))
	}
	for _, want := range []string{
		`ec2 = "http://127.0.0.1:4566/ec2/region/eu-west-1"`,
		`sts = "http://127.0.0.1:4566/sts"`,
		fmt.Sprintf("s3 = %q", rehearsalDeadProxy),
		fmt.Sprintf("elbv2 = %q", rehearsalDeadProxy),
	} {
		assert.Equal(t, 3, countOf(lines, want), "%q in each of the 3 blocks\n%s", want, override)
	}
	assert.NotContains(t, override, "{region}")
}

// Each block, the default's included, holds its alias, the walls env's
// keys, skip_metadata_api_check and every endpoints key. A "" alias names
// the default block, as the gate's alias list does.
func TestRehearsalProviderOverrideBlocks(t *testing.T) {
	t.Parallel()

	override := rehearsalProviderOverride(overrideTestFakeaws, overrideTestRegion, overrideTestKeys, append([]string{""}, overrideTestAliases...))
	env := rehearsalWallsEnv("/rehearsal", overrideTestFakeaws, overrideTestRegion)

	f, diags := hclsyntax.ParseConfig(override, ourOverride, hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())
	var aliases []string
	for _, block := range f.Body.(*hclsyntax.Body).Blocks {
		require.Equal(t, "provider", block.Type)
		require.Equal(t, []string{"aws"}, block.Labels)
		attrs := block.Body.Attributes
		alias := ""
		if attrs["alias"] != nil {
			alias = overrideValue(t, attrs["alias"]).AsString()
		}
		aliases = append(aliases, alias)
		assert.Equal(t, env["AWS_ACCESS_KEY_ID"], overrideValue(t, attrs["access_key"]).AsString(), alias)
		assert.Equal(t, env["AWS_SECRET_ACCESS_KEY"], overrideValue(t, attrs["secret_key"]).AsString(), alias)
		assert.True(t, overrideValue(t, attrs["skip_metadata_api_check"]).True(), alias)
		require.Len(t, block.Body.Blocks, 1, alias)
		assert.Equal(t, "endpoints", block.Body.Blocks[0].Type, alias)
		assert.Len(t, block.Body.Blocks[0].Body.Attributes, len(overrideTestKeys), alias)
	}
	assert.Equal(t, []string{"", "east", "west"}, aliases)
}

// With no fakeaws URL no endpoint is relative: every one is dead, as
// rehearsalWallsEnv sends every service.
func TestRehearsalProviderOverrideWithoutFakeawsIsDead(t *testing.T) {
	t.Parallel()

	override := rehearsalProviderOverride(" ", overrideTestRegion, overrideTestKeys, nil)

	for _, line := range strings.Split(string(override), "\n") {
		key, value, _ := strings.Cut(line, "=")
		if key = strings.TrimSpace(key); slices.Contains(overrideTestKeys, key) {
			assert.Equal(t, fmt.Sprintf("%q", rehearsalDeadProxy), strings.TrimSpace(value), key)
		}
	}
}

// The modelled keys are exactly the services rehearsalWallsEnv sends to
// fakeaws: every awsFakeawsPaths service but S3.
func TestRehearsalModelledEndpointsAreFakeawsServices(t *testing.T) {
	t.Parallel()

	services := slices.DeleteFunc(slices.Collect(maps.Keys(awsFakeawsPaths)), func(svc string) bool { return svc == "S3" })
	assert.ElementsMatch(t, services, slices.Collect(maps.Values(rehearsalModelledEndpoints)))
	assert.NotContains(t, rehearsalModelledEndpoints, "s3")
}

// overrideTestSchema is `tofu providers schema -json` for hashicorp/aws
// 5.100.0, trimmed to a few provider attributes, two of its nested blocks
// and six of its endpoints keys, plus a second provider. %[1]s is the aws
// entry's host.
const overrideTestSchema = `{
  "format_version": "1.0",
  "provider_schemas": {
    "%[1]s/hashicorp/aws": {
      "provider": {
        "version": 0,
        "block": {
          "attributes": {
            "access_key": {"type": "string", "description_kind": "plain", "optional": true},
            "region": {"type": "string", "description_kind": "plain", "optional": true}
          },
          "block_types": {
            "assume_role": {"nesting_mode": "list", "block": {"attributes": {"role_arn": {"type": "string", "description_kind": "plain", "optional": true}}, "description_kind": "plain"}},
            "endpoints": {"nesting_mode": "set", "block": {"attributes": {
              "sts": {"type": "string", "description": "Use this to override the default service endpoint URL", "description_kind": "plain", "optional": true},
              "s3": {"type": "string", "description": "Use this to override the default service endpoint URL", "description_kind": "plain", "optional": true},
              "elbv2": {"type": "string", "description": "Use this to override the default service endpoint URL", "description_kind": "plain", "optional": true},
              "ec2": {"type": "string", "description": "Use this to override the default service endpoint URL", "description_kind": "plain", "optional": true},
              "route53": {"type": "string", "description": "Use this to override the default service endpoint URL", "description_kind": "plain", "optional": true},
              "accessanalyzer": {"type": "string", "description": "Use this to override the default service endpoint URL", "description_kind": "plain", "optional": true}
            }, "description_kind": "plain"}}
          },
          "description_kind": "plain"
        }
      }
    },
    "registry.opentofu.org/hashicorp/random": {"provider": {"version": 0, "block": {"description_kind": "plain"}}}
  }
}`

func TestRehearsalEndpointKeys(t *testing.T) {
	t.Parallel()

	for _, host := range []string{"registry.opentofu.org", "registry.terraform.io"} {
		keys, err := rehearsalEndpointKeys(fmt.Appendf(nil, overrideTestSchema, host))
		require.NoError(t, err, host)
		assert.Equal(t, []string{"accessanalyzer", "ec2", "elbv2", "route53", "s3", "sts"}, keys, host)
	}
}

func TestRehearsalEndpointKeysRefuses(t *testing.T) {
	t.Parallel()

	opentofu := fmt.Sprintf(overrideTestSchema, "registry.opentofu.org")
	for name, tc := range map[string]struct{ schema, want string }{
		"no endpoints block": {
			strings.Replace(opentofu, `"endpoints"`, `"not_endpoints"`, 1),
			"the registry.opentofu.org/hashicorp/aws schema has no endpoints block"},
		"no sts key": {
			strings.Replace(opentofu, `"sts"`, `"not_sts"`, 1),
			"the registry.opentofu.org/hashicorp/aws endpoints block names no sts key"},
		"no hashicorp/aws entry": {
			fmt.Sprintf(overrideTestSchema, "registry.example.com"),
			"the provider schema names 0 hashicorp/aws providers ()"},
		"both hosts": {
			strings.Replace(opentofu, `"provider_schemas": {`, `"provider_schemas": {"registry.terraform.io/hashicorp/aws": {},`, 1),
			"the provider schema names 2 hashicorp/aws providers (registry.opentofu.org/hashicorp/aws, registry.terraform.io/hashicorp/aws)"},
		"not JSON": {"tofu printed a warning first", "the provider schema does not parse"},
	} {
		keys, err := rehearsalEndpointKeys([]byte(tc.schema))
		assert.Nil(t, keys, name)
		if assert.Error(t, err, name) {
			assert.Contains(t, err.Error(), tc.want, name)
		}
	}
}

func overrideValue(t *testing.T, attr *hclsyntax.Attribute) cty.Value {
	t.Helper()
	require.NotNil(t, attr)
	value, diags := attr.Expr.Value(nil)
	require.False(t, diags.HasErrors(), diags.Error())
	return value
}
