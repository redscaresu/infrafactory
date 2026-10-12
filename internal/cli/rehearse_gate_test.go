package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gateFixture writes files (path relative to the copy: content) into a
// fresh copy and gates its root, plus "child" as a module dir when any
// file is under it.
func gateFixture(t *testing.T, files map[string]string, varFile string) error {
	t.Helper()
	copyDir := t.TempDir()
	in := rehearsalGateInput{root: copyDir, boundary: copyDir, region: "us-east-1"}
	for name, content := range files {
		path := filepath.Join(copyDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	if _, err := os.Stat(filepath.Join(copyDir, "child")); err == nil {
		in.modules = []string{filepath.Join(copyDir, "child")}
	}
	if varFile != "" {
		in.varFile = filepath.Join(t.TempDir(), "vars.tfvars")
		require.NoError(t, os.WriteFile(in.varFile, []byte(varFile), 0o600))
	}
	return rehearsalGate(in)
}

const encryption = `terraform {
  encryption {
    key_provider "pbkdf2" "k" {
      passphrase = "correct-horse-battery-staple"
    }
  }
}
`

func TestRehearsalGateRefuses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		files   map[string]string
		varFile string
		want    []string
	}{
		// Rule 1: the file policy.
		{name: "encryption in a .tf file", files: map[string]string{"main.tf": encryption},
			want: []string{"main.tf:2: terraform.encryption: refused"}},
		{name: "a .tofu file", files: map[string]string{"main.tofu": encryption},
			want: []string{"main.tofu: .tofu files are refused by name"}},
		{name: "a .tf.json file", files: map[string]string{"main.tf.json": `{"terraform": {"encryption": {}}}`},
			want: []string{"main.tf.json: .tf.json files are refused by name"}},
		{name: "a .tftest.hcl file", files: map[string]string{"main.tftest.hcl": encryption},
			want: []string{"main.tftest.hcl: .tftest.hcl files are refused by name"}},
		{name: "every other refused suffix", files: map[string]string{
			"a.tofu.json": "{}", "a.tfvars.json": "{}", "a.tftest.json": "{}",
			"a.tofutest.hcl": "", "a.tofutest.json": "{}", "a.tfmock.hcl": "", "B.TOFU": "",
		}, want: []string{
			"a.tofu.json: .tofu.json files", "a.tfvars.json: .tfvars.json files", "a.tftest.json: .tftest.json files",
			"a.tofutest.hcl: .tofutest.hcl files", "a.tofutest.json: .tofutest.json files", "a.tfmock.hcl: .tfmock.hcl files",
			"B.TOFU: .tofu files",
		}},
		{name: "a terraform.d dir", files: map[string]string{"terraform.d/plugins/x": ""},
			want: []string{"terraform.d: a terraform.d entry is refused"}},
		{name: "a tests dir, in the root and a child", files: map[string]string{
			"tests/evil.tftest.hcl":    "run \"r\" {\n  module {\n    source = \"git::https://example.com/m.git\"\n  }\n}\n",
			"child/Tests/a.tftest.hcl": "",
		}, want: []string{"tests: a tests entry is refused", "child/Tests: a tests entry is refused"}},
		{name: "an unparseable .tf", files: map[string]string{"broken.tf": "resource \"aws_vpc\" \"v\" {\n"},
			want: []string{"broken.tf: does not parse"}},
		{name: "a .tfvars reference", files: map[string]string{"terraform.tfvars": "region = var.x\n"},
			want: []string{"terraform.tfvars:1: region: a .tfvars value must be a literal"}},
		{name: "a .tfvars call and block", files: map[string]string{"x.tfvars": "a = \"ok\"\nb = file(\"/etc/passwd\")\nc {}\n"},
			want: []string{"x.tfvars:2: b: a .tfvars value must be a literal", "x.tfvars:3: c: a .tfvars file holds only attributes"}},
		{name: "a var file reference", varFile: "region = local.r\n",
			want: []string{"vars.tfvars:1: region: a .tfvars value must be a literal"}},
		{name: "top-level attributes", files: map[string]string{"main.tf": "x = core::file(\"/etc/passwd\")\ny = timestamp()\n"},
			want: []string{"main.tf:1: x: refused: a .tf file holds only blocks", "main.tf:2: y: refused: a .tf file holds only blocks"}},
		// Rule 2: override files.
		{name: "zz_override.tf", files: map[string]string{"zz_override.tf": "locals {}\n"},
			want: []string{"zz_override.tf: an override file is refused"}},
		{name: "infrafactory_override.tf", files: map[string]string{"infrafactory_override.tf": "locals {}\n"},
			want: []string{"infrafactory_override.tf: an override file is refused"}},
		{name: "override.tf", files: map[string]string{"Override.tf": "locals {}\n"},
			want: []string{"Override.tf: an override file is refused"}},
		// Rule 3: terraform {}.
		{name: "cloud", files: map[string]string{"main.tf": "terraform {\n  cloud {}\n}\n"},
			want: []string{"main.tf:2: terraform.cloud: refused"}},
		{name: "an unnamed terraform attribute", files: map[string]string{"main.tf": "terraform {\n  experiments = []\n}\n"},
			want: []string{"main.tf:2: terraform.experiments: refused"}},
		{name: "a backend in a child module", files: map[string]string{"child/main.tf": "terraform {\n  backend \"s3\" {}\n}\n"},
			want: []string{"child/main.tf:2: terraform.backend.s3: refused"}},
		// Rule 4: top-level blocks.
		{name: "data http inside check", files: map[string]string{"main.tf": "check \"c\" {\n  data \"http\" \"h\" {\n    url = \"https://example.com\"\n  }\n}\n"},
			want: []string{`main.tf:1: check.c: "check" blocks are refused`}},
		{name: "ephemeral", files: map[string]string{"main.tf": "ephemeral \"aws_secretsmanager_secret_version\" \"s\" {}\n"},
			want: []string{`main.tf:1: ephemeral.aws_secretsmanager_secret_version.s: "ephemeral" blocks are refused`}},
		// Rule 5: provider sources and local names.
		{name: "a foreign source", files: map[string]string{"main.tf": "terraform {\n  required_providers {\n    aws = { source = \"evil/aws\" }\n  }\n}\n"},
			want: []string{`main.tf:3: terraform.required_providers.aws: source "evil/aws" is not admitted`}},
		{name: "a host-qualified source", files: map[string]string{"main.tf": "terraform {\n  required_providers {\n    aws = { source = \"evil.example/hashicorp/aws\" }\n  }\n}\n"},
			want: []string{`main.tf:3: terraform.required_providers.aws: source "evil.example/hashicorp/aws" is not admitted`}},
		{name: "an aws source under another local name, at root", files: map[string]string{"main.tf": aliasedAWS},
			want: []string{
				`main.tf:3: terraform.required_providers.x: local name "x" differs from its source's type "aws"`,
				"main.tf:7: provider.x.endpoints: refused",
			}},
		{name: "an aws source under another local name, in a child", files: map[string]string{"child/main.tf": aliasedAWS},
			want: []string{
				`child/main.tf:3: terraform.required_providers.x: local name "x" differs from its source's type "aws"`,
				`child/main.tf:6: provider.x: a provider "aws" block in a child module is refused`,
			}},
		{name: "an implied provider", files: map[string]string{"main.tf": "provider \"google\" {}\n"},
			want: []string{`main.tf:1: provider.google: provider source "hashicorp/google" is not admitted`}},
		{name: "a provider meta-argument", files: map[string]string{"main.tf": "resource \"aws_vpc\" \"v\" {\n  provider = google.x\n}\n"},
			want: []string{`main.tf:2: aws_vpc.v.provider: provider source "hashicorp/google" is not admitted`}},
		// Rule 6: resource and data types.
		{name: "data http", files: map[string]string{"main.tf": "data \"http\" \"h\" {}\n"},
			want: []string{`main.tf:1: data.http.h: data type "http" is not admitted`}},
		{name: "data external", files: map[string]string{"main.tf": "data \"external\" \"e\" {}\n"},
			want: []string{`main.tf:1: data.external.e: data type "external" is not admitted`}},
		{name: "terraform_remote_state", files: map[string]string{"main.tf": "data \"terraform_remote_state\" \"s\" {}\n"},
			want: []string{`main.tf:1: data.terraform_remote_state.s: data type "terraform_remote_state" is not admitted`}},
		{name: "data tls_certificate", files: map[string]string{"main.tf": "data \"tls_certificate\" \"c\" {}\n"},
			want: []string{`main.tf:1: data.tls_certificate.c: data type "tls_certificate" is not admitted`}},
		{name: "data local_command", files: map[string]string{"main.tf": "data \"local_command\" \"c\" {}\n"},
			want: []string{`main.tf:1: data.local_command.c: data type "local_command" is not admitted`}},
		{name: "random_made_up", files: map[string]string{"main.tf": "resource \"random_made_up\" \"r\" {}\n"},
			want: []string{`main.tf:1: random_made_up.r: resource type "random_made_up" is not admitted`}},
		// Rule 7: provisioner and connection at any depth.
		{name: "provisioner", files: map[string]string{"main.tf": "resource \"terraform_data\" \"t\" {\n  provisioner \"local-exec\" {\n    command = \"id\"\n  }\n}\n"},
			want: []string{`main.tf:2: terraform_data.t.provisioner.local-exec: "provisioner" blocks are refused`}},
		{name: "connection in a removed block", files: map[string]string{"main.tf": "removed {\n  from = aws_instance.a\n  provisioner \"remote-exec\" {\n    connection {\n      host = \"h\"\n    }\n  }\n}\n"},
			want: []string{
				`main.tf:3: removed.provisioner.remote-exec: "provisioner" blocks are refused`,
				`main.tf:4: removed.provisioner.remote-exec.connection: "connection" blocks are refused`,
			}},
		// Rule 8: provider "aws" in a child module.
		{name: "provider aws in a child", files: map[string]string{"child/main.tf": "provider \"aws\" {}\n"},
			want: []string{`child/main.tf:1: provider.aws: a provider "aws" block in a child module is refused`}},
		// Rule 9: other providers admit only alias.
		{name: "a tls provider argument", files: map[string]string{"main.tf": "provider \"tls\" {\n  proxy {\n    url = \"http://p\"\n  }\n}\nprovider \"random\" {\n  alias = \"a\"\n  x     = 1\n}\n"},
			want: []string{"main.tf:2: provider.tls.proxy: refused: a tls provider block admits only alias", "main.tf:8: provider.random.x: refused: a random provider block admits only alias"}},
		// Rule 10: functions.
		{name: "core::file", files: map[string]string{"main.tf": "output \"o\" {\n  value = core::file(\"/etc/passwd\")\n}\n"},
			want: []string{"main.tf:2: output.o.value: calls core::file(): a namespaced function is refused by name"}},
		{name: "provider::local::direxists", files: map[string]string{"main.tf": "locals {\n  d = provider::local::direxists(\"/\")\n}\n"},
			want: []string{"main.tf:2: locals.d: calls provider::local::direxists(): a namespaced function is refused by name"}},
		{name: "a nested impure call", files: map[string]string{"main.tf": "resource \"aws_vpc\" \"v\" {\n  tags = {\n    at = timestamp()\n  }\n  lifecycle {\n    precondition {\n      condition     = uuid() != \"\"\n      error_message = \"x\"\n    }\n  }\n}\n"},
			want: []string{"main.tf:2: aws_vpc.v.tags: calls timestamp(), which is not on the pure-function allowlist", "main.tf:7: aws_vpc.v.lifecycle.precondition.condition: calls uuid()"}},
		// The closed rules.
		{name: "provider aws region", files: map[string]string{"main.tf": "provider \"aws\" {\n  region = \"us-east-1\"\n}\n"},
			want: []string{`main.tf:2: provider.aws.region: refused: a provider "aws" block admits only alias`}},
		{name: "file()", files: map[string]string{"main.tf": "output \"o\" {\n  value = file(\"${path.module}/a.txt\")\n}\n"},
			want: []string{"main.tf:2: output.o.value: calls file(), which reads a path: the gate admits no path"}},
		{name: "a local_file path", files: map[string]string{"main.tf": "resource \"local_file\" \"f\" {\n  content  = \"x\"\n  filename = \"/tmp/x\"\n}\n"},
			want: []string{"main.tf:3: local_file.f.filename: refused: the gate admits no path"}},
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

// aliasedAWS declares the aws source under the local name x, and an x
// provider block that sets endpoints.
const aliasedAWS = `terraform {
  required_providers {
    x = { source = "hashicorp/aws" }
  }
}
provider "x" {
  endpoints {}
}
`

// TestRehearsalGateAdmits: every construct the gate admits, in one copy,
// passes. The local and archive types carry no path argument: the path
// rule is closed.
func TestRehearsalGateAdmits(t *testing.T) {
	err := gateFixture(t, map[string]string{
		"main.tf": `terraform {
  required_version = ">= 1.8"
  required_providers {
    aws       = { source = "hashicorp/aws", version = "5.100.0" }
    random    = { source = "hashicorp/random" }
    null      = { source = "hashicorp/null" }
    tls       = { source = "HashiCorp/TLS" }
    time      = { source = "hashicorp/time" }
    cloudinit = { source = "hashicorp/cloudinit" }
    local     = { source = "hashicorp/local" }
    archive   = "~> 2.0"
  }
  backend "s3" {
    bucket = "state"
    key    = "prod.tfstate"
    region = "us-east-1"
  }
}

provider "aws" {}

provider "aws" {
  alias = "west"
}

provider "random" {
  alias = "second"
}

variable "names" {
  type    = list(string)
  default = ["a", "b"]
}

locals {
  prefix = upper("app")
}

data "aws_caller_identity" "me" {
  provider = aws.west
}

resource "aws_s3_bucket" "logs" {
  for_each = toset(var.names)
  bucket   = "${local.prefix}-${each.key}"

  lifecycle {
    prevent_destroy = true
  }
}

resource "random_id" "suffix" {
  count       = 2
  byte_length = 4
}

resource "null_resource" "noop" {}

resource "tls_private_key" "key" {
  algorithm = "ED25519"
}

resource "time_static" "now" {}

data "cloudinit_config" "boot" {
  part {
    content = "#!/bin/sh"
  }
}

resource "local_file" "out" {
  content = "x"
}

data "archive_file" "zip" {
  type = "zip"
}

resource "terraform_data" "marker" {
  input = random_id.suffix[0].hex
}

moved {
  from = aws_s3_bucket.old
  to   = aws_s3_bucket.logs
}

import {
  to = aws_s3_bucket.logs["a"]
  id = "app-a"
}

removed {
  from = aws_s3_bucket.gone

  lifecycle {
    destroy = false
  }
}

output "account" {
  value = data.aws_caller_identity.me.account_id
}
`,
		"terraform.tfvars": "names = [\"a\", \"b\"]\n",
		"README.md":        "# Not HCL {\n",
		"child/main.tf": `terraform {
  required_providers {
    random = { source = "hashicorp/random" }
  }
}

resource "random_pet" "p" {}
`,
	}, "names = [\"c\"]\n")
	assert.NoError(t, err)
}
