package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// awsRoot is a root with one aws provider whose region is region, a
// region variable defaulting to def, and a resource using it.
func awsRoot(region, def string) string {
	return "variable \"region\" {\n  default = " + def + "\n}\nprovider \"aws\" {\n  region = " + region + "\n}\nresource \"aws_vpc\" \"v\" {}\n"
}

const noDefaultProvider = `the root module: refused: it has no provider "aws" block without an alias`

func TestRehearsalGateAWSRefuses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		files   map[string]string
		varFile string
		want    []string
	}{
		// The provider "aws" table.
		{name: "endpoints, assume_role, profile and https_proxy", files: map[string]string{"main.tf": `provider "aws" {
  region      = "us-east-1"
  profile     = "prod"
  https_proxy = "http://p"
  endpoints {
    sts = "http://x"
  }
  assume_role {
    role_arn = "arn:aws:iam::123456789012:role/r"
  }
}
`}, want: []string{
			`main.tf:3: provider.aws.profile: refused: a provider "aws" block admits only alias, allowed_account_ids, default_tags,`,
			`main.tf:4: provider.aws.https_proxy: refused: a provider "aws" block admits only`,
			`main.tf:5: provider.aws.endpoints: refused: a provider "aws" block admits only`,
			`main.tf:8: provider.aws.assume_role: refused: a provider "aws" block admits only`,
		}},
		{name: "an argument the table does not name", files: map[string]string{"main.tf": "provider \"aws\" {\n  region      = \"us-east-1\"\n  made_up_arg = 1\n}\n"},
			want: []string{"main.tf:3: provider.aws.made_up_arg: refused: the gate's table of hashicorp/aws " + harness.AWSProviderVersion + " provider arguments does not name it"}},
		// Goal 11: the provider's region.
		{name: "a --var-file overriding a matching default", files: map[string]string{"main.tf": awsRoot("var.region", `"us-east-1"`)}, varFile: "region = \"eu-west-1\"\n",
			want: []string{`main.tf:5: provider.aws.region: refused: region "eu-west-1" (var.region from --var-file) is not --region "us-east-1"`}},
		{name: "another literal", files: map[string]string{"main.tf": awsRoot(`"eu-west-1"`, "null")},
			want: []string{`main.tf:5: provider.aws.region: refused: region "eu-west-1" is not --region "us-east-1"`}},
		{name: "a local", files: map[string]string{"main.tf": awsRoot("local.region", "null") + "locals {\n  region = \"us-east-1\"\n}\n"},
			want: []string{"main.tf:5: provider.aws.region: refused: region must be a string literal or var.<name> (it reads local.region)"}},
		{name: "a function call", files: map[string]string{"main.tf": awsRoot(`lower("US-EAST-1")`, "null")},
			want: []string{"main.tf:5: provider.aws.region: refused: region must be a string literal or var.<name> (it reads no reference, but does not evaluate as a literal)"}},
		{name: "an aliased provider with another region", files: map[string]string{"main.tf": awsRoot(`"us-east-1"`, "null") + "provider \"aws\" {\n  alias  = \"west\"\n  region = \"eu-west-1\"\n}\n"},
			want: []string{`main.tf:10: provider.aws.region: refused: region "eu-west-1" is not --region "us-east-1"`}},
		{name: "a provider with no region", files: map[string]string{"main.tf": "provider \"aws\" {\n  alias = \"west\"\n}\n"},
			want: []string{`main.tf:1: provider.aws: refused: a provider "aws" block must set region`}},
		{name: "a variable with no value", files: map[string]string{"main.tf": "variable \"region\" {}\nprovider \"aws\" {\n  region = var.region\n}\n"},
			want: []string{"main.tf:3: provider.aws.region: refused: var.region has no value"}},
		{name: "a null default", files: map[string]string{"main.tf": awsRoot("var.region", "null")},
			want: []string{"main.tf:5: provider.aws.region: refused: var.region takes its value from its default, which is not a string literal"}},
		{name: "terraform.tfvars over a matching default", files: map[string]string{"main.tf": awsRoot("var.region", `"us-east-1"`), "terraform.tfvars": "region = \"eu-west-1\"\n"},
			want: []string{`region "eu-west-1" (var.region from terraform.tfvars) is not --region`}},
		{name: "an auto.tfvars over a matching terraform.tfvars", files: map[string]string{
			"main.tf": awsRoot("var.region", "null"), "terraform.tfvars": "region = \"us-east-1\"\n", "a.auto.tfvars": "region = \"eu-west-1\"\n",
		}, want: []string{`region "eu-west-1" (var.region from a.auto.tfvars) is not --region`}},
		{name: "the later auto.tfvars", files: map[string]string{
			"main.tf": awsRoot("var.region", "null"), "a.auto.tfvars": "region = \"us-east-1\"\n", "b.auto.tfvars": "region = \"eu-west-1\"\n",
		}, want: []string{`region "eu-west-1" (var.region from b.auto.tfvars) is not --region`}},
		{name: "a var file spelled in another case", files: map[string]string{
			"main.tf": awsRoot("var.region", `"us-east-1"`), "Terraform.tfvars": "region = \"eu-west-1\"\n", "B.AUTO.tfvars": "region = \"eu-west-1\"\n",
		}, want: []string{
			"Terraform.tfvars: refused: tofu loads terraform.tfvars and *.auto.tfvars by name, and the gate cannot say whether it loads this spelling",
			"B.AUTO.tfvars: refused: tofu loads terraform.tfvars",
		}},
		// Goal 11: a root with no unaliased provider "aws" block, whatever it holds.
		{name: "aws_* types with no provider", files: map[string]string{"main.tf": "resource \"aws_vpc\" \"v\" {}\ndata \"aws_caller_identity\" \"me\" {}\n"},
			want: []string{noDefaultProvider}},
		{name: "aws_* types with only an aliased provider", files: map[string]string{"main.tf": "provider \"aws\" {\n  alias  = \"west\"\n  region = \"us-east-1\"\n}\nresource \"aws_vpc\" \"v\" {\n  provider = aws.west\n}\n"},
			want: []string{noDefaultProvider}},
		{name: "aws_* types in a child with no provider", files: map[string]string{
			"main.tf": "module \"c\" {\n  source = \"./child\"\n}\n", "child/main.tf": "data \"aws_caller_identity\" \"me\" {}\n",
		}, want: []string{noDefaultProvider}},
		{name: "only a removed block naming an aws_* address", files: map[string]string{"main.tf": "removed {\n  from = aws_s3_bucket.old\n  lifecycle {\n    destroy = true\n  }\n}\n"},
			want: []string{noDefaultProvider}},
		{name: "no aws_* address at all", files: map[string]string{"main.tf": manifestOutput},
			want: []string{noDefaultProvider}},
		// Goal 11: a region argument on an aws_* block.
		{name: "a resource's region", files: map[string]string{"main.tf": awsRoot(`"us-east-1"`, "null") + "resource \"aws_s3_bucket\" \"b\" {\n  region = \"eu-west-1\"\n}\n"},
			want: []string{`main.tf:9: aws_s3_bucket.b.region: refused: region "eu-west-1" is not --region "us-east-1"`}},
		{name: "a data block's region from a variable", files: map[string]string{"main.tf": awsRoot(`"us-east-1"`, `"eu-west-1"`) + "data \"aws_region\" \"r\" {\n  region = var.region\n}\n"},
			want: []string{`main.tf:9: data.aws_region.r.region: refused: region "eu-west-1" (var.region from its default) is not --region "us-east-1"`}},
		{name: "a variable region in a module", files: map[string]string{
			"main.tf":       awsRoot(`"us-east-1"`, "null") + "module \"c\" {\n  source = \"./child\"\n}\n",
			"child/main.tf": "variable \"region\" {\n  default = \"us-east-1\"\n}\nresource \"aws_s3_bucket\" \"b\" {\n  region = var.region\n}\n",
		}, want: []string{`module "c": child/main.tf:5: aws_s3_bucket.b.region: refused: var.region is in a module, and the gate resolves variables only in the root`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := gateFixture(t, tc.files, tc.varFile)
			require.ErrorIs(t, err, errRehearsalRefuses)
			for _, want := range tc.want {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

// TestRehearsalGateAWSNoRegionIsTheOnlyRefusal: an unaliased provider
// "aws" block with no region is still the default, so aws_* types beside
// it draw only the missing-region refusal.
func TestRehearsalGateAWSNoRegionIsTheOnlyRefusal(t *testing.T) {
	err := gateFixture(t, map[string]string{"main.tf": "provider \"aws\" {}\nresource \"aws_vpc\" \"v\" {}\n"}, "")
	assert.EqualError(t, err, errRehearsalRefuses.Error()+`: main.tf:1: provider.aws: refused: a provider "aws" block must set region, so goal 11 can check it`)
}

// TestRehearsalGateAWSUnparsedRootIsNotNamedProviderless: a root .tf the
// gate cannot parse is refused, and since it may hold the default provider
// block, the gate does not also call the root providerless.
func TestRehearsalGateAWSUnparsedRootIsNotNamedProviderless(t *testing.T) {
	err := gateFixture(t, map[string]string{"provider.tf": "provider \"aws\" {\n  region = \"us-east-1\"\n"}, "")
	require.ErrorIs(t, err, errRehearsalRefuses)
	assert.Contains(t, err.Error(), "provider.tf: does not parse")
	assert.NotContains(t, err.Error(), noDefaultProvider)
}

func TestRehearsalGateAWSAdmits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		files   map[string]string
		varFile string
	}{
		{name: "every admitted argument", files: map[string]string{"main.tf": `provider "aws" {
  region                      = "us-east-1"
  allowed_account_ids         = ["123456789012"]
  forbidden_account_ids       = ["210987654321"]
  max_retries                 = 1
  retry_mode                  = "standard"
  s3_use_path_style           = true
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  default_tags {
    tags = { team = "a" }
  }
  ignore_tags {
    keys = ["x"]
  }
}
`}},
		{name: "a matching literal, aliased too", files: map[string]string{"main.tf": awsRoot(`"us-east-1"`, "null") + "provider \"aws\" {\n  alias  = \"east\"\n  region = \"us-east-1\"\n}\n"}},
		{name: "a matching default, wrapped in a template", files: map[string]string{"main.tf": awsRoot(`"${var.region}"`, `"us-east-1"`)}},
		{name: "a matching terraform.tfvars over another default", files: map[string]string{"main.tf": awsRoot("var.region", `"eu-west-1"`), "terraform.tfvars": "region = \"us-east-1\"\n"}},
		{name: "a matching auto.tfvars over another terraform.tfvars", files: map[string]string{
			"main.tf": awsRoot("var.region", `"eu-west-1"`), "terraform.tfvars": "region = \"eu-west-1\"\n",
			"a.auto.tfvars": "region = \"eu-west-1\"\n", "b.auto.tfvars": "region = \"us-east-1\"\n", "prod.tfvars": "region = \"eu-west-1\"\n",
		}},
		{name: "a matching --var-file over another auto.tfvars", files: map[string]string{
			"main.tf": awsRoot("var.region", `"eu-west-1"`), "z.auto.tfvars": "region = \"eu-west-1\"\n",
		}, varFile: "region = \"us-east-1\"\n"},
		{name: "matching regions on aws_* blocks, in a module too", files: map[string]string{
			"main.tf":       awsRoot(`"us-east-1"`, `"us-east-1"`) + "data \"aws_region\" \"r\" {\n  region = var.region\n}\nmodule \"c\" {\n  source = \"./child\"\n}\n",
			"child/main.tf": "resource \"aws_s3_bucket\" \"b\" {\n  region = \"us-east-1\"\n}\n",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.NoError(t, gateFixture(t, tc.files, tc.varFile))
		})
	}
}
