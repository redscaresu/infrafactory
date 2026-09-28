package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/harness"
)

const awsGateTestRunID = "20260928T000000Z"

// awsWiredStack is the e2e web-step-one fixture after ensureAwsProviderWiring
// for us-east-1, keyed by file name.
func awsWiredStack(t *testing.T, runID string) map[string]string {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("..", "e2e", "testdata", "aws-web-step-one", "web-step-one.tf"))
	require.NoError(t, err)
	files := map[string][]byte{"main.tf": fixture}
	var cfg config.Config
	cfg.AWS.Region = "us-east-1"
	require.NoError(t, ensureAwsProviderWiring(files, cfg, runID))
	stack := make(map[string]string, len(files))
	for name, content := range files {
		stack[name] = string(content)
	}
	return stack
}

// awsProviderProblemsIn parses files the way the gate does and runs
// awsProviderProblems over them.
func awsProviderProblemsIn(t *testing.T, files map[string]string, region string) []string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	parsed, varDefaults, err := layer3ParseDir(dir)
	require.NoError(t, err)
	return awsProviderProblems(parsed, varDefaults, region)
}

func TestAWSProviderProblemsAdmitsTheWiringInfrafactoryWrites(t *testing.T) {
	for _, runID := range []string{awsGateTestRunID, ""} {
		t.Run("runID="+runID, func(t *testing.T) {
			stack := awsWiredStack(t, runID)
			assert.Equal(t, runID != "", strings.Contains(stack["providers.tf"], awsRunIDTagKey), "default_tags is written only with a run id")
			assert.Empty(t, awsProviderProblemsIn(t, stack, "us-east-1"))

			stack["versions.tf"] = `terraform { required_version = ">= 1.6" }`
			assert.Empty(t, awsProviderProblemsIn(t, stack, "us-east-1"))
		})
	}

	stack := awsWiredStack(t, awsGateTestRunID)
	stack["providers.tf"] = strings.Replace(stack["providers.tf"], `"us-east-1"`, "var.r", 1)
	stack["variables.tf"] = `variable "r" { default = "us-east-1" }`
	assert.Empty(t, awsProviderProblemsIn(t, stack, "us-east-1"), "a region variable whose default is the configured region is admitted")
}

func TestAWSProviderProblemsRefuses(t *testing.T) {
	pinned := awsRequiredProviders(`aws = {
      source  = "hashicorp/aws"
      version = "` + harness.AWSProviderVersion + `"
    }`)
	provider := buildAwsProviderBlock("us-east-1", awsGateTestRunID)
	untagged := buildAwsProviderBlock("us-east-1", "")
	with := func(block, extra string) string { return strings.TrimSuffix(block, "}") + extra + "}\n" }
	otherResource := func(body string) string {
		return "resource \"aws_vpc\" \"other\" {\n  cidr_block = \"10.1.0.0/16\"\n" + body + "}\n"
	}
	encryption := func(inner string) string { return "terraform {\n  encryption {\n" + inner + "  }\n}\n" }

	type refusal struct {
		files    map[string]string
		noRegion bool
		want     string
	}
	cases := map[string]refusal{
		// terraform {}, through layer3TerraformBlockProblems.
		"external key provider": {files: map[string]string{"extra.tf": encryption("    key_provider \"external\" \"k\" {\n      command = [\"sh\", \"-c\", \"id\"]\n    }\n")},
			want: `extra.tf: terraform block "encryption" is not permitted`},
		"external method": {files: map[string]string{"extra.tf": encryption("    method \"external\" \"m\" {\n      encrypt_command = [\"id\"]\n      decrypt_command = [\"id\"]\n    }\n")},
			want: `extra.tf: terraform block "encryption" is not permitted`},
		"aws_kms key provider": {files: map[string]string{"extra.tf": encryption("    key_provider \"aws_kms\" \"k\" {\n      kms_key_id = \"alias/state\"\n      key_spec   = \"AES_256\"\n    }\n")},
			want: `extra.tf: terraform block "encryption" is not permitted`},
		"provider_meta": {files: map[string]string{"extra.tf": "terraform {\n  provider_meta \"aws\" {\n  }\n}\n"},
			want: `extra.tf: terraform block "provider_meta" is not permitted`},
		"backend s3": {files: map[string]string{"extra.tf": "terraform {\n  backend \"s3\" {\n    bucket = \"elsewhere\"\n  }\n}\n"},
			want: `extra.tf: terraform block "backend" is not permitted`},
		"cloud": {files: map[string]string{"extra.tf": "terraform {\n  cloud {\n    organization = \"elsewhere\"\n  }\n}\n"},
			want: `extra.tf: terraform block "cloud" is not permitted`},
		"experiments":   {files: map[string]string{"extra.tf": "terraform { experiments = [] }\n"}, want: `extra.tf: terraform setting "experiments" is not permitted`},
		"unknown block": {files: map[string]string{"extra.tf": "terraform {\n  anything {\n  }\n}\n"}, want: `extra.tf: terraform block "anything" is not permitted`},

		// The provider block.
		"no provider":              {files: map[string]string{"providers.tf": pinned}, want: `no provider "aws" block`},
		"two providers, one file":  {files: map[string]string{"providers.tf": pinned + provider + "\n" + provider}, want: `2 provider "aws" blocks (providers.tf:9, providers.tf:19)`},
		"two providers, two files": {files: map[string]string{"extra.tf": provider}, want: `2 provider "aws" blocks (extra.tf:1, providers.tf:10)`},
		"provider scaleway":        {files: map[string]string{"extra.tf": "provider \"scaleway\" {}\n"}, want: `extra.tf: provider "scaleway" is not permitted`},
		"unlabelled provider":      {files: map[string]string{"extra.tf": "provider {}\n"}, want: `extra.tf: a provider block must be labelled exactly "aws"`},
		"no region":                {files: map[string]string{"providers.tf": pinned + "provider \"aws\" {\n  s3_use_path_style = true\n}\n"}, want: `providers.tf: provider "aws" sets no region`},
		"another region": {files: map[string]string{"providers.tf": pinned + buildAwsProviderBlock("eu-west-1", awsGateTestRunID)},
			want: `providers.tf: provider "aws" region "eu-west-1" is not the configured aws.region "us-east-1"`},
		"region var without default": {files: map[string]string{"providers.tf": pinned + strings.Replace(provider, `"us-east-1"`, "var.r", 1), "variables.tf": "variable \"r\" {}\n"},
			want: `providers.tf: provider "aws" region is not a literal`},
		"empty configured region": {noRegion: true, want: `aws.region is not configured`},
		"path style false": {files: map[string]string{"providers.tf": pinned + strings.Replace(provider, "s3_use_path_style = true", "s3_use_path_style = false", 1)},
			want: `providers.tf: provider "aws" s3_use_path_style must be literal true`},
		"assume_role block": {files: map[string]string{"providers.tf": pinned + with(provider, "  assume_role {\n    role_arn = \"arn:aws:iam::123456789012:role/x\"\n  }\n")},
			want: `providers.tf: provider "aws" block "assume_role" is not permitted`},
		"endpoints":   {files: map[string]string{"providers.tf": pinned + with(provider, "  endpoints {\n  }\n")}, want: `providers.tf: provider "aws" block "endpoints" is not permitted`},
		"ignore_tags": {files: map[string]string{"providers.tf": pinned + with(provider, "  ignore_tags {\n  }\n")}, want: `providers.tf: provider "aws" block "ignore_tags" is not permitted`},
		"default_tags second key": {files: map[string]string{"providers.tf": pinned + with(untagged, "  default_tags {\n    tags = {\n      \"infrafactory-run-id\" = \"r\"\n      owner = \"x\"\n    }\n  }\n")},
			want: `providers.tf: provider "aws" default_tags sets tag "owner"`},
		"default_tags non-literal value": {files: map[string]string{"providers.tf": pinned + with(untagged, "  default_tags {\n    tags = {\n      \"infrafactory-run-id\" = var.run\n    }\n  }\n")},
			want: `providers.tf: provider "aws" default_tags tag "infrafactory-run-id" must be a literal string`},
		"default_tags twice": {files: map[string]string{"providers.tf": pinned + with(provider, "  default_tags {\n    tags = {\n      \"infrafactory-run-id\" = \"r\"\n    }\n  }\n")},
			want: `providers.tf: provider "aws" has 2 default_tags blocks`},
		"run-id tag on a resource": {files: map[string]string{"extra.tf": otherResource("  tags = {\n    \"infrafactory-run-id\" = \"another-run\"\n  }\n")},
			want: `extra.tf: resource "aws_vpc" "other" names the "infrafactory-run-id" tag`},
		"run-id tag as a bare key": {files: map[string]string{"extra.tf": otherResource("  tags = {\n    infrafactory-run-id = \"another-run\"\n  }\n")},
			want: `extra.tf: resource "aws_vpc" "other" names the "infrafactory-run-id" tag`},
		"provider meta-argument": {files: map[string]string{"extra.tf": otherResource("  provider = aws.x\n")},
			want: `extra.tf: resource "aws_vpc" "other" sets provider`},

		// required_providers, through the pinned layer3ProviderSourceProblems.
		"source attacker/aws": {files: map[string]string{"providers.tf": awsRequiredProviders(`aws = { source = "attacker/aws", version = "`+harness.AWSProviderVersion+`" }`) + provider},
			want: `providers.tf: required_provider "aws" must be source "hashicorp/aws"`},
		"version range": {files: map[string]string{"providers.tf": awsRequiredProviders(`aws = { source = "hashicorp/aws", version = "~> 5.100" }`) + provider},
			want: `providers.tf: required_provider "aws" pins version "~> 5.100"`},
		"another exact version": {files: map[string]string{"providers.tf": awsRequiredProviders(`aws = { source = "hashicorp/aws", version = "5.99.0" }`) + provider},
			want: `providers.tf: required_provider "aws" pins version "5.99.0"`},
		"no version": {files: map[string]string{"providers.tf": awsRequiredProviders(`aws = { source = "hashicorp/aws" }`) + provider},
			want: `providers.tf: required_provider "aws" declares no version`},
		"configuration_aliases": {files: map[string]string{"providers.tf": awsRequiredProviders(`aws = { source = "hashicorp/aws", version = "`+harness.AWSProviderVersion+`", configuration_aliases = [aws.x] }`) + provider},
			want: `providers.tf: required_provider "aws" is not a literal object`},
		"configuration_aliases literal": {files: map[string]string{"providers.tf": awsRequiredProviders(`aws = { source = "hashicorp/aws", version = "`+harness.AWSProviderVersion+`", configuration_aliases = [] }`) + provider},
			want: `providers.tf: required_provider "aws" sets "configuration_aliases"; only source and version are permitted`},
		"a random provider": {files: map[string]string{"providers.tf": pinned + provider, "extra.tf": awsRequiredProviders(`random = { source = "hashicorp/random", version = "3.6.0" }`)},
			want: `extra.tf: required_provider "random" must be source "hashicorp/aws"`},
		"no required_providers": {files: map[string]string{"providers.tf": provider},
			want: `no terraform.required_providers entry declares source "hashicorp/aws"`},
	}
	for _, attr := range []string{"alias", "access_key", "secret_key", "token", "profile", "shared_config_files", "shared_credentials_files", "skip_credentials_validation", "assume_role"} {
		cases["attribute "+attr] = refusal{files: map[string]string{"providers.tf": pinned + with(provider, "  "+attr+" = \"x\"\n")},
			want: `providers.tf: provider setting "` + attr + `" is not permitted`}
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			stack := awsWiredStack(t, awsGateTestRunID)
			for file, content := range tc.files {
				stack[file] = content
			}
			region := "us-east-1"
			if tc.noRegion {
				region = ""
			}
			assert.Contains(t, strings.Join(awsProviderProblemsIn(t, stack, region), "\n"), tc.want)
		})
	}
}

func awsRequiredProviders(entries string) string {
	return "terraform {\n  required_providers {\n    " + entries + "\n  }\n}\n"
}
