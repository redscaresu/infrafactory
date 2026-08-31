package cli

import (
	"slices"
	"strings"
	"testing"

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
		"aws":      {"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION"},
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
		{"aws", func(f map[string][]byte) { ensureAwsProviderWiring(f, cfg) }, validateAwsProviderWiring},
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

// TestEnsureAwsProviderWiringInjectsEndpoints asserts the AWS
// counterpart of the historically-untested GCP+Scaleway injection
// path: when AWS resources are present AND cfg.Fakeaws.URL is set,
// the injected provider block contains skip_credentials_validation
// = true AND an endpoints { } block. Without those, the
// terraform-provider-aws SDK escapes to real AWS STS — the failure
// mode this whole parity slice was built to prevent.
func TestEnsureAwsProviderWiringInjectsEndpoints(t *testing.T) {
	t.Parallel()
	cfg := config.Config{
		Fakeaws: config.FakeawsConfig{URL: "http://127.0.0.1:8082"},
		S3:      config.S3Config{URL: "http://127.0.0.1:9090"},
	}
	files := map[string][]byte{
		"main.tf": []byte(`resource "aws_sqs_queue" "jobs" { name = "jobs" }`),
	}
	ensureAwsProviderWiring(files, cfg)

	providers, ok := files["providers.tf"]
	if !ok {
		t.Fatal("ensureAwsProviderWiring did not produce providers.tf when an aws resource is present")
	}
	body := string(providers)
	for _, must := range []string{
		`provider "aws"`,
		`skip_credentials_validation = true`,
		`skip_metadata_api_check     = true`,
		`skip_requesting_account_id  = true`,
		`endpoints {`,
		`http://127.0.0.1:8082/iam`,
		`http://127.0.0.1:9090`, // S3 endpoint
	} {
		if !strings.Contains(body, must) {
			t.Errorf("injected providers.tf missing required substring %q\nfull body:\n%s", must, body)
		}
	}
}

// TestEnsureGoogleProviderWiringInjectsCustomEndpoints — the GCP
// counterpart to TestEnsureAwsProviderWiringInjectsEndpoints. The
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

	layer3, err := sandboxCommandEnvForProject(runtime, "")
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
