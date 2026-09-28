package cli

import (
	"context"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
)

// modelAWSProviderTF is a providers.tf a model might write: the old
// endpoints block with skip_*, an aliased second block, and a loose pin.
const modelAWSProviderTF = `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
    random = {
      source = "hashicorp/random"
    }
  }
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_requesting_account_id  = true
  endpoints {
    sqs = "http://127.0.0.1:8082/sqs/region/us-east-1"
  }
}

provider "aws" {
  alias  = "west"
  region = "us-west-2"
}
`

const sqsQueueTF = `resource "aws_sqs_queue" "jobs" {
  name = "jobs"
}
`

const testRunID = "20260928T101500Z"

func wireAWS(t *testing.T, model map[string][]byte, cfg config.Config, runID string) map[string]string {
	t.Helper()
	files := maps.Clone(model)
	require.NoError(t, ensureAwsProviderWiring(files, cfg, runID))
	out := make(map[string]string, len(files))
	for name, content := range files {
		out[name] = string(content)
	}
	return out
}

// Layer-neutral means the bytes the wiring writes do not depend on where
// the run applies: not on Layer 3, not on which mocks are configured. The
// run id is the one value that differs between runs, and only as the
// default_tags value.
func TestEnsureAwsProviderWiringIsLayerNeutral(t *testing.T) {
	t.Parallel()
	model := map[string][]byte{"main.tf": []byte(sqsQueueTF), "providers.tf": []byte(modelAWSProviderTF)}
	want := wireAWS(t, model, config.Config{}, testRunID)
	require.Contains(t, want["providers.tf"], `"infrafactory-run-id" = "`+testRunID+`"`)

	for _, layer3 := range []bool{false, true} {
		for _, fakeaws := range []string{"", "http://127.0.0.1:8082"} {
			for _, s3 := range []string{"", "http://127.0.0.1:9090"} {
				cfg := config.Config{Fakeaws: config.FakeawsConfig{URL: fakeaws}, S3: config.S3Config{URL: s3}}
				cfg.Validation.Layers.SandboxDeploy.Enabled = layer3
				assert.Equal(t, want, wireAWS(t, model, cfg, testRunID), "sandbox_deploy=%t fakeaws=%q s3=%q", layer3, fakeaws, s3)
			}
		}
	}

	otherRun := maps.Clone(want)
	otherRun["providers.tf"] = strings.Replace(want["providers.tf"], testRunID, "20260928T111500Z", 1)
	assert.Equal(t, otherRun, wireAWS(t, model, config.Config{}, "20260928T111500Z"), "the run id changes the tag value and nothing else")

	regional := wireAWS(t, model, config.Config{AWS: config.AWSConfig{Region: "eu-west-2"}}, testRunID)
	want["providers.tf"] = strings.Replace(want["providers.tf"], `"us-east-1"`, `"eu-west-2"`, 1)
	assert.Equal(t, want, regional, "aws.region changes the region literal and nothing else")
}

// requiredProviders is every required_providers entry across files, by
// name; a name declared twice has two values.
func requiredProviders(t *testing.T, files map[string][]byte) map[string][]cty.Value {
	t.Helper()
	entries := map[string][]cty.Value{}
	for name, content := range files {
		file, diags := hclsyntax.ParseConfig(content, name, hcl.InitialPos)
		require.False(t, diags.HasErrors(), "%s: %s\n%s", name, diags, content)
		for _, terraform := range file.Body.(*hclsyntax.Body).Blocks {
			if terraform.Type != "terraform" {
				continue
			}
			for _, required := range terraform.Body.Blocks {
				if required.Type != "required_providers" {
					continue
				}
				for key, attr := range required.Body.Attributes {
					value, diags := attr.Expr.Value(nil)
					require.False(t, diags.HasErrors(), "%s: %s", name, diags)
					entries[key] = append(entries[key], value)
				}
			}
		}
	}
	return entries
}

func TestEnsureAwsProviderWiringPinsTheExactVersion(t *testing.T) {
	t.Parallel()
	pin := cty.ObjectVal(map[string]cty.Value{
		"source":  cty.StringVal("hashicorp/aws"),
		"version": cty.StringVal(harness.AWSProviderVersion),
	})
	random := cty.ObjectVal(map[string]cty.Value{"source": cty.StringVal("hashicorp/random")})

	for name, entry := range map[string]string{
		"~> 5.70":          `aws = { source = "hashicorp/aws", version = "~> 5.70" }`,
		">= 4.0":           `aws = { source = "hashicorp/aws", version = ">= 4.0" }`,
		"5.100.0":          `aws = { source = "hashicorp/aws", version = "5.100.0" }`,
		"no source":        `aws = { version = "~> 5.70" }`,
		"missing entry":    ``,
		"legacy shorthand": `aws = "~> 5.70"`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := map[string][]byte{
				"main.tf": []byte(sqsQueueTF),
				"versions.tf": []byte("terraform {\n  required_providers {\n    random = { source = \"hashicorp/random\" }\n    " +
					entry + "\n  }\n}\n"),
			}

			require.NoError(t, ensureAwsProviderWiring(files, config.Config{}, testRunID))

			assert.Equal(t, map[string][]cty.Value{"aws": {pin}, "random": {random}}, requiredProviders(t, files))
			require.NoError(t, validateAwsProviderWiring(files))
		})
	}
}

// awsProviderVersionRe is "hashicorp/aws" followed, within a line or two
// and with no digit between, by a version. The gap keeps any constraint
// operator (~>, >=) so it can be refused.
var awsProviderVersionRe = regexp.MustCompile(`hashicorp/aws([^0-9]{0,40}?)(\d+\.\d+(?:\.\d+)?)`)

// The model is told the pin, the reviewer checks against it, and the docs
// name it: each must be the one generation writes.
func TestEveryAWSProviderVersionIsThePin(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	prompts, err := filepath.Glob(filepath.Join(root, "prompts", "aws", "*.md"))
	require.NoError(t, err)
	require.NotEmpty(t, prompts)
	paths := append(prompts, filepath.Join(root, "README.md"))
	require.NoError(t, filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			paths = append(paths, path)
		}
		return nil
	}))

	matches := 0
	for _, path := range paths {
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.False(t, strings.Contains(string(content), "~> 5.70"), "%s names ~> 5.70", path)
		for _, m := range awsProviderVersionRe.FindAllStringSubmatch(string(content), -1) {
			matches++
			assert.Equal(t, harness.AWSProviderVersion, m[2], "%s: %q", path, m[0])
			assert.False(t, strings.ContainsAny(m[1], "~<>!"), "%s: %q is a range, not the pin", path, m[0])
		}
	}
	assert.GreaterOrEqual(t, matches, len(prompts)+1, "every AWS prompt and the README name the pin")
}

const layer2AWSScenarioYAML = `scenario: layer2-aws-env
version: "1.0"
cloud: aws
description: An SQS queue.
resources:
  networking:
    vpc: true
acceptance_criteria:
  - type: policy
    check: region_restriction
    params:
      region: us-east-1
    expect: pass
`

// Without skip_* in the provider block, STS is called at configure, so
// every tofu command a Layer 2 AWS run starts must carry the STS endpoint
// and the catch-all, from init to destroy.
func TestLayer2AWSRunSetsEndpointsOnEveryTofuCommand(t *testing.T) {
	t.Parallel()
	recorder := &recordingRunner{stdout: []byte("{}")}
	cfg := config.Default()
	cfg.Fakeaws.URL = "http://127.0.0.1:8082"
	cfg.Paths.Output = t.TempDir()
	cfg.Paths.Policies = filepath.Join("..", "..", "policies")
	rt := &CommandRuntime{
		Config:         cfg,
		Logger:         NewAppLogger(io.Discard),
		scenarioLoader: defaultScenarioLoader,
		Deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(context.Context, generator.Request) (*generator.GeneratedCode, error) {
				return &generator.GeneratedCode{Files: map[string][]byte{"main.tf": []byte(sqsQueueTF)}}, nil
			}),
			Static:     harness.NewStaticHarness(recorder),
			MockDeploy: harness.NewMockDeployHarness(recorder, stubMockStateClient{}),
			Destroy:    harness.NewDestroyHarness(recorder, stubMockStateClient{}),
		},
	}
	ctx, scenarioPath := context.Background(), writeScenarioForTest(t, layer2AWSScenarioYAML)
	_, err := rt.LoadScenario(scenarioPath)
	require.NoError(t, err)

	_, _, err = generateAndWriteFilesWithResult(ctx, rt, scenarioPath, "", 1, nil, generatedFileWriteModeClean)
	require.NoError(t, err)
	_, _, err = executeValidateWithArtifacts(ctx, rt, scenarioPath)
	require.NoError(t, err)
	_, err = executeTest(ctx, rt, scenarioPath, testExecutionOptions{Progress: io.Discard})
	require.NoError(t, err)

	var args []string
	for _, cmd := range recorder.commands {
		args = append(args, cmd.Args[0])
		assert.Equal(t, cfg.Fakeaws.URL+"/sts", cmd.Env["AWS_ENDPOINT_URL_STS"], "tofu %v", cmd.Args)
		assert.Equal(t, awsDeadEndpointURL, cmd.Env["AWS_ENDPOINT_URL"], "tofu %v", cmd.Args)
	}
	assert.Equal(t, []string{"init", "validate", "plan", "show", "init", "apply", "plan", "destroy"}, args,
		"static init, validate, plan, show; mock init, apply, converge plan; destroy")
}
