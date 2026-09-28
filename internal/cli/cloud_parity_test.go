package cli

import (
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

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// TestCloudEnvCoversAllThreeClouds asserts that cloudEnv sets the
// minimum credential + endpoint env vars terraform-provider-{scaleway,
// google,aws} each need to talk to a mock server. The original test/
// validate harness only set Scaleway env vars — that asymmetry is
// what made AWS+GCP LLM-driven runs silently escape to real cloud
// APIs.
//
// Pinning the required keys here means a future env-var rename or
// removal that breaks any cloud surfaces immediately in CI rather
// than in a customer's `infrafactory run` log.
func TestCloudEnvCoversAllThreeClouds(t *testing.T) {
	t.Parallel()
	runtime := &CommandRuntime{Config: config.Config{
		Mockway: config.MockwayConfig{URL: "http://127.0.0.1:8080"},
	}}
	env := cloudEnv(runtime)

	// Per-cloud required env-var sets. If a future commit drops one,
	// the test fails with a specific message naming the cloud + key.
	required := map[string][]string{
		"scaleway": {"SCW_API_URL", "SCW_ACCESS_KEY", "SCW_SECRET_KEY", "SCW_DEFAULT_PROJECT_ID"},
		"gcp":      {"GOOGLE_OAUTH_ACCESS_TOKEN", "GOOGLE_PROJECT"},
		"aws": append([]string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION",
			"AWS_EC2_METADATA_DISABLED", "AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE"}, layer2AWSEndpointKeys...),
	}

	for cloud, keys := range required {
		for _, k := range keys {
			v, ok := env[k]
			if !ok {
				t.Errorf("cloudEnv missing required %s key %q — terraform-provider-%s won't reach its mock backend without it",
					strings.ToUpper(cloud), k, cloud)
				continue
			}
			if v == "" {
				t.Errorf("cloudEnv[%q] is empty (%s) — must be a non-empty placeholder", k, cloud)
			}
		}
	}
}

// TestEnsureProviderWiringCoverageIsSymmetric asserts every supported
// cloud has an ensure*ProviderWiring + detect* + validate* triple.
// Today: Scaleway, GCP, AWS. The smoke-test pattern is to call each
// function with an empty file map and confirm it doesn't panic — the
// real behavior is exercised by per-cloud unit tests adjacent to the
// implementation.
//
// This catches the M64-era gap where Scaleway + GCP had ensure*
// helpers but AWS didn't, so LLM-generated AWS HCL never got the
// test-mode provider block injected.
func TestEnsureProviderWiringCoverageIsSymmetric(t *testing.T) {
	t.Parallel()
	cfg := config.Config{}
	emptyFiles := func() map[string][]byte {
		return map[string][]byte{"main.tf": []byte{}}
	}

	// Each entry is "cloud key -> wiring + validate functions".
	cases := []struct {
		cloud    string
		wireFn   func(map[string][]byte)
		validate func(map[string][]byte) error
	}{
		{"scaleway", ensureScalewayProviderWiring, validateScalewayProviderWiring},
		{"gcp", func(f map[string][]byte) { ensureGoogleProviderWiring(f, cfg) }, validateGoogleProviderWiring},
		{"aws", func(f map[string][]byte) { require.NoError(t, ensureAwsProviderWiring(f, cfg, testRunID)) }, validateAwsProviderWiring},
	}

	for _, tc := range cases {
		t.Run(tc.cloud, func(t *testing.T) {
			files := emptyFiles()
			tc.wireFn(files)
			// With no resource of that cloud's prefix in main.tf, the
			// wire-fn should be a no-op — no providers.tf created.
			if _, ok := files["providers.tf"]; ok {
				t.Errorf("%s wire-fn injected providers.tf with no matching resources in HCL", tc.cloud)
			}
			if err := tc.validate(files); err != nil {
				t.Errorf("%s validate-fn rejected an empty file set: %v", tc.cloud, err)
			}
		})
	}
}

// TestEnsureAwsProviderWiringWritesOnlyRegionPathStyleAndRunTag is the
// inverse of the endpoints block this wiring used to inject: with fakeaws
// and S3 both configured, the one provider "aws" block left is region,
// s3_use_path_style and default_tags carrying the run id, with no
// endpoints, skip_*, keys or alias. Endpoints come from cloudEnv at
// Layer 2 and from nowhere at Layer 3 (ADR-0039).
func TestEnsureAwsProviderWiringWritesOnlyRegionPathStyleAndRunTag(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		Fakeaws: config.FakeawsConfig{URL: "http://127.0.0.1:8082"},
		S3:      config.S3Config{URL: "http://127.0.0.1:9090"},
		AWS:     config.AWSConfig{Region: "eu-west-2"},
	}
	files := map[string][]byte{
		"main.tf":      []byte(`resource "aws_sqs_queue" "jobs" { name = "jobs" }` + "\n"),
		"providers.tf": []byte(modelAWSProviderTF),
	}

	require.NoError(t, ensureAwsProviderWiring(files, cfg, testRunID))

	var blocks []*hclsyntax.Block
	for name, content := range files {
		file, diags := hclsyntax.ParseConfig(content, name, hcl.InitialPos)
		require.False(t, diags.HasErrors(), "%s: %s\n%s", name, diags, content)
		for _, block := range file.Body.(*hclsyntax.Body).Blocks {
			if block.Type == "provider" && slices.Equal(block.Labels, []string{"aws"}) {
				blocks = append(blocks, block)
			}
		}
	}
	require.Len(t, blocks, 1, "the wiring's block must be the only provider \"aws\" block")
	body := blocks[0].Body
	require.Len(t, body.Blocks, 1, "no nested block but default_tags: no endpoints")
	defaultTags := body.Blocks[0].Body
	assert.Equal(t, "default_tags", body.Blocks[0].Type)
	assert.Empty(t, defaultTags.Blocks)
	require.ElementsMatch(t, []string{"tags"}, slices.Collect(maps.Keys(defaultTags.Attributes)))
	tags, diags := defaultTags.Attributes["tags"].Expr.Value(nil)
	require.False(t, diags.HasErrors())
	assert.Equal(t, cty.ObjectVal(map[string]cty.Value{"infrafactory-run-id": cty.StringVal(testRunID)}), tags)
	assert.ElementsMatch(t, []string{"region", "s3_use_path_style"}, slices.Collect(maps.Keys(body.Attributes)))
	region, diags := body.Attributes["region"].Expr.Value(nil)
	require.False(t, diags.HasErrors())
	assert.Equal(t, cty.StringVal("eu-west-2"), region)
	pathStyle, diags := body.Attributes["s3_use_path_style"].Expr.Value(nil)
	require.False(t, diags.HasErrors())
	assert.Equal(t, cty.True, pathStyle)
}

// TestEnsureGoogleProviderWiringInjectsCustomEndpoints: unlike AWS,
// terraform-provider-google takes its endpoints from the block. The
// bare `provider "google" {}` injection that lived in this file
// before the parity work was insufficient (no *_custom_endpoint
// overrides means every API call escaped to api.googleapis.com).
func TestEnsureGoogleProviderWiringInjectsCustomEndpoints(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		Fakegcp: config.FakegcpConfig{URL: "http://127.0.0.1:8081"},
	}
	files := map[string][]byte{
		"main.tf": []byte(`resource "google_pubsub_topic" "t" { name = "t" }`),
	}
	ensureGoogleProviderWiring(files, cfg)

	providers, ok := files["providers.tf"]
	if !ok {
		t.Fatal("ensureGoogleProviderWiring did not produce providers.tf when a google resource is present")
	}
	body := string(providers)
	// Every per-service *_custom_endpoint we ship in the injection
	// must be present + must point at the fakegcp base URL.
	requiredEndpoints := []string{
		`compute_custom_endpoint`,
		`container_custom_endpoint`,
		`cloud_resource_manager_custom_endpoint`,
		`iam_custom_endpoint`,
		`storage_custom_endpoint`,
		`sql_custom_endpoint`,
		`pubsub_custom_endpoint`,
		`dns_custom_endpoint`,
		`cloud_run_v2_custom_endpoint`,
		`secret_manager_custom_endpoint`,
		`service_usage_custom_endpoint`,
	}
	for _, ep := range requiredEndpoints {
		if !strings.Contains(body, ep) {
			t.Errorf("injected providers.tf missing %s — terraform-provider-google won't route this service to fakegcp", ep)
		}
	}
	if !strings.Contains(body, "http://127.0.0.1:8081") {
		t.Errorf("injected providers.tf doesn't reference cfg.Fakegcp.URL\n%s", body)
	}
}

// TestLayer2AndLayer3ScalewayEnvsAreInverses pins the single most
// important invariant Layer 3 depends on: the two layers must disagree
// about SCW_API_URL.
//
// Layer 2 sets it, to keep the apply on mockway. Layer 3 must both omit
// it AND strip any inherited value, so the provider falls through to
// its real default. The parity test above guards the Layer 2 half; if
// the two ever agree, one of the layers is applying to the wrong place
// -- and the failure mode is silent, because a mock-targeted "real"
// apply still reports pass.
func TestLayer2AndLayer3ScalewayEnvsAreInverses(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SCW_ACCESS_KEY", "real-access")
	t.Setenv("SCW_SECRET_KEY", "real-secret")
	t.Setenv("SCW_DEFAULT_ORGANIZATION_ID", "22222222-2222-2222-2222-222222222222")

	runtime := &CommandRuntime{Config: config.Config{
		Mockway:  config.MockwayConfig{URL: "http://127.0.0.1:8080"},
		Scaleway: config.ScalewayConfig{Region: "fr-par", Zone: "fr-par-1"},
	}}

	layer2 := cloudEnv(runtime)
	if layer2["SCW_API_URL"] != "http://127.0.0.1:8080" {
		t.Fatalf("Layer 2 must point SCW_API_URL at mockway, got %q", layer2["SCW_API_URL"])
	}

	layer3, err := sandboxEnvWithProjectDefault(runtime, "")
	if err != nil {
		t.Fatalf("sandboxCommandEnv: %v", err)
	}
	if _, ok := layer3["SCW_API_URL"]; ok {
		t.Fatal("Layer 3 must not set SCW_API_URL — the provider default is the real endpoint")
	}
	if !slices.Contains(harness.SandboxStripEnv, "SCW_API_URL") {
		t.Fatal("Layer 3 must strip an inherited SCW_API_URL; omitting it from the override map is not enough")
	}
}

// The AWS form of the invariant above: Layer 2 names an endpoint for
// every service, Layer 3 names none and strips any it inherits, so the
// provider falls through to real AWS only at Layer 3.
func TestLayer2AndLayer3AWSEnvsAreInverses(t *testing.T) {
	t.Parallel()

	layer2 := cloudEnv(&CommandRuntime{Config: config.Config{Fakeaws: config.FakeawsConfig{URL: "http://127.0.0.1:8082"}}})
	for _, key := range layer2AWSEndpointKeys {
		assert.NotEmpty(t, layer2[key], "Layer 2 must set %s", key)
	}

	credFile := filepath.Join(t.TempDir(), "layer3-aws.env")
	require.NoError(t, os.WriteFile(credFile, []byte("AWS_ACCESS_KEY_ID=AKIAFROMTHEFILE00001\nAWS_SECRET_ACCESS_KEY=s\n"), 0o600))
	layer3, err := harness.AWSSealedEnv(credFile, "eu-west-2")
	require.NoError(t, err)
	for key := range layer3 {
		assert.False(t, strings.HasPrefix(key, "AWS_ENDPOINT_URL"), "Layer 3 must not set %s", key)
	}

	assert.Contains(t, harness.SandboxStripEnv, "AWS_*", "Layer 3 must strip an inherited AWS_ENDPOINT_URL*")
}
