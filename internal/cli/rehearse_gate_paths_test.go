package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gateOutput is a main.tf whose line 2 sets output.o.value to expr.
func gateOutput(expr string) map[string]string {
	return map[string]string{"main.tf": "output \"o\" {\n  value = " + expr + "\n}\n"}
}

// gateLocalFile is a main.tf holding one resource "local_file" "f" whose
// line 3 sets arg to the HCL value val.
func gateLocalFile(arg, val string) map[string]string {
	return map[string]string{"main.tf": "resource \"local_file\" \"f\" {\n  content = \"x\"\n  " + arg + " = " + val + "\n}\n"}
}

func TestRehearsalGatePathsRefuse(t *testing.T) {
	withTemplate := func(tpl string) map[string]string {
		files := gateOutput(`templatefile("${path.module}/t.tpl", {})`)
		files["t.tpl"] = tpl
		return files
	}
	for _, tc := range []struct {
		name   string
		files  map[string]string
		want   []string
		absent []string
	}{
		// Path functions.
		{name: "an absolute path", files: gateOutput(`file("/etc/passwd")`),
			want: []string{"main.tf:2: output.o.value: file(): an absolute path is refused"}},
		{name: "an escaping bare literal", files: gateOutput(`file("../../x")`),
			want: []string{"main.tf:2: output.o.value: file(): a bare relative path is refused"}},
		{name: "a bare relative literal", files: gateOutput(`file("files/a.txt")`),
			want: []string{"main.tf:2: output.o.value: file(): a bare relative path is refused"}},
		{name: "path.module escaping the copy", files: gateOutput(`file("${path.module}/../../x")`),
			want: []string{"main.tf:2: output.o.value: file(): the path resolves outside the copy"}},
		{name: "a child's path.module escaping the copy", files: map[string]string{"child/main.tf": "locals {\n  x = filemd5(\"${path.module}/../../x\")\n}\n"},
			want: []string{"child/main.tf:2: locals.x: filemd5(): the path resolves outside the copy"}},
		{name: "a variable path", files: gateOutput(`templatefile(var.p, {})`),
			want: []string{"main.tf:2: output.o.value: templatefile(): a variable or computed path is refused"}},
		{name: "path.module with no separator", files: gateOutput(`file("${path.module}x")`),
			want: []string{"main.tf:2: output.o.value: file(): a variable or computed path is refused"}},
		{name: "path.cwd", files: gateOutput(`file("${path.cwd}/x")`),
			want: []string{"main.tf:2: output.o.value: file(): a variable or computed path is refused"}},
		{name: "a nested path call", files: gateOutput(`base64encode(filesha256(pathexpand("~/.aws/credentials")))`),
			want: []string{
				"main.tf:2: output.o.value: calls base64encode(), which is not on the pure-function allowlist",
				"main.tf:2: output.o.value: filesha256(): a variable or computed path is refused",
			}},
		{name: "pathexpand", files: gateOutput(`pathexpand("~/.aws/credentials")`),
			want: []string{"main.tf:2: output.o.value: calls pathexpand(), which is not on the pure-function allowlist"}},
		{name: "abspath", files: gateOutput(`abspath(path.module)`),
			want: []string{"main.tf:2: output.o.value: calls abspath(), which is not on the pure-function allowlist"}},
		{name: "core::templatefile", files: gateOutput(`core::templatefile("${path.module}/t.tpl", {})`),
			want:   []string{"main.tf:2: output.o.value: calls core::templatefile(): a namespaced function is refused by name"},
			absent: []string{"templatefile(): template"}},
		// fileset's glob.
		{name: "a glob climbing to home", files: gateOutput(`fileset(path.module, "../../../../home/*/.aws/*")`),
			want: []string{"main.tf:2: output.o.value: fileset(): a glob holding a .. element is refused"}},
		{name: "a glob with .. after **", files: gateOutput(`fileset(path.module, "**/../x")`),
			want: []string{"main.tf:2: output.o.value: fileset(): a glob holding a .. element is refused"}},
		{name: "a glob with an escaped ..", files: gateOutput(`fileset(path.module, "a/.\\./b")`),
			want: []string{"main.tf:2: output.o.value: fileset(): a glob may hold only letters, digits and / . - _ * ?"}},
		{name: "a glob spelling .. with braces", files: gateOutput(`fileset(path.module, "{..,.}/{..,.}/home/*/.aws/*")`),
			want: []string{"main.tf:2: output.o.value: fileset(): a glob may hold only letters, digits and / . - _ * ?"}},
		{name: "a glob spelling .. with classes", files: gateOutput(`fileset(path.module, "[.][.]/x")`),
			want: []string{"main.tf:2: output.o.value: fileset(): a glob may hold only letters, digits and / . - _ * ?"}},
		{name: "a variable glob", files: gateOutput(`fileset(path.module, var.p)`),
			want: []string{"main.tf:2: output.o.value: fileset(): the glob must be a literal"}},
		{name: "an absolute glob", files: gateOutput(`fileset(path.module, "/etc/*")`),
			want: []string{"main.tf:2: output.o.value: fileset(): an absolute glob is refused"}},
		// Templates.
		{name: "a template reading an absolute path", files: withTemplate(`${file("/etc/passwd")}`),
			want: []string{"main.tf:2: output.o.value: templatefile(): template t.tpl: calls file(): a path in a template is refused"}},
		{name: "a template whose path.module the vars map sets", files: map[string]string{
			"main.tf": "output \"o\" {\n  value = templatefile(\"${path.module}/t.tpl\", { path = { module = \"/Users/me/.aws\" } })\n}\n",
			"t.tpl":   `${file("${path.module}/credentials")}`,
		}, want: []string{"main.tf:2: output.o.value: templatefile(): template t.tpl: calls file(): a path in a template is refused"}},
		{name: "a template calling an impure function", files: withTemplate("at ${timestamp()}\n"),
			want: []string{"main.tf:2: output.o.value: templatefile(): template t.tpl: calls timestamp(), which is not on the pure-function allowlist"}},
		{name: "a template calling templatefile", files: withTemplate(`${templatefile("${path.module}/t.tpl", {})}`),
			want: []string{"main.tf:2: output.o.value: templatefile(): template t.tpl: calls templatefile(): a path in a template is refused"}},
		{name: "a template that does not parse", files: withTemplate("${"),
			want: []string{"main.tf:2: output.o.value: templatefile(): template t.tpl does not parse"}},
		{name: "a missing template", files: gateOutput(`templatefile("${path.root}/t.tpl", {})`),
			want: []string{"main.tf:2: output.o.value: templatefile(): template t.tpl: cannot read it"}},
		// Path arguments.
		{name: "local_file filename absolute", files: gateLocalFile("filename", `"/tmp/x"`),
			want: []string{"main.tf:3: local_file.f.filename: an absolute path is refused"}},
		{name: "local_file source absolute", files: gateLocalFile("source", `"/etc/passwd"`),
			want: []string{"main.tf:3: local_file.f.source: an absolute path is refused"}},
		{name: "archive_file source_dir escaping", files: map[string]string{"main.tf": "data \"archive_file\" \"a\" {\n  type       = \"zip\"\n  source_dir = \"../..\"\n}\n"},
			want: []string{"main.tf:3: data.archive_file.a.source_dir: a bare relative path is refused"}},
		{name: "data local_file filename computed", files: map[string]string{"main.tf": "data \"local_file\" \"l\" {\n  filename = var.p\n}\n"},
			want: []string{"main.tf:2: data.local_file.l.filename: a variable or computed path is refused"}},
		// Written paths.
		{name: "writing an override file", files: gateLocalFile("filename", `"${path.root}/zz_override.tf"`),
			want: []string{"main.tf:3: local_file.f.filename: a written path ending .tf is refused"}},
		{name: "writing a .TOFU file", files: gateLocalFile("filename", `"${path.module}/x.TOFU"`),
			want: []string{"main.tf:3: local_file.f.filename: a written path ending .tofu is refused"}},
		{name: "writing a .tfvars file", files: gateLocalFile("filename", `"${path.module}/sub/x.tfvars"`),
			want: []string{"main.tf:3: local_file.f.filename: a written path ending .tfvars is refused"}},
		{name: "writing the lock file", files: gateLocalFile("filename", `"${path.root}/.terraform.lock.hcl"`),
			want: []string{"main.tf:3: local_file.f.filename: a written path is refused: tofu reads it"}},
		{name: "writing under .terraform", files: gateLocalFile("filename", `"${path.root}/.Terraform/x"`),
			want: []string{"main.tf:3: local_file.f.filename: a written path is refused: tofu reads it"}},
		{name: "writing under terraform.d", files: map[string]string{"main.tf": "resource \"archive_file\" \"a\" {\n  type        = \"zip\"\n  output_path = \"${path.root}/terraform.d/x.zip\"\n}\n"},
			want: []string{"main.tf:3: archive_file.a.output_path: a written path is refused: tofu reads it"}},
		{name: "writing a template a templatefile reads", files: map[string]string{
			"main.tf": "resource \"local_file\" \"t\" {\n  content  = \"x\"\n  filename = \"${path.module}/t.tpl\"\n}\noutput \"o\" {\n  value = templatefile(\"${path.module}/t.tpl\", { v = local_file.t.id })\n}\n",
			"t.tpl":   "hello\n",
		}, want: []string{"main.tf:3: local_file.t.filename: refused: the apply would overwrite template t.tpl, which a templatefile reads in the same apply"}},
		{name: "a child writing the root's template", files: map[string]string{
			"main.tf":       "output \"o\" {\n  value = templatefile(\"${path.root}/t.tpl\", {})\n}\n",
			"t.tpl":         "hello\n",
			"child/main.tf": "data \"archive_file\" \"a\" {\n  type        = \"zip\"\n  output_path = \"${path.module}/../t.tpl\"\n}\n",
		}, want: []string{"child/main.tf:3: data.archive_file.a.output_path: refused: the apply would overwrite template t.tpl"}},
		{name: "a module block's dir writing the root's template", files: map[string]string{
			"main.tf":     "module \"m\" {\n  source = \"./mod\"\n}\noutput \"o\" {\n  value = templatefile(\"${path.root}/t.tpl\", {})\n}\n",
			"t.tpl":       "hello\n",
			"mod/main.tf": "data \"archive_file\" \"a\" {\n  type        = \"zip\"\n  output_path = \"${path.module}/../t.tpl\"\n}\n",
		}, want: []string{`module "m": mod/main.tf:3: data.archive_file.a.output_path: refused: the apply would overwrite template t.tpl`}},
		{name: "an absolute path in a module block's dir", files: map[string]string{
			"main.tf":     "module \"m\" {\n  source = \"./mod\"\n}\n",
			"mod/main.tf": "data \"local_file\" \"l\" {\n  filename = \"/etc/passwd\"\n}\n",
		}, want: []string{`module "m": mod/main.tf:2: data.local_file.l.filename: an absolute path is refused`}},
		// aws path arguments: every argument terraform-provider-aws v5.100.0
		// reads a local file from.
		{name: "aws_s3_object source", files: map[string]string{"main.tf": "resource \"aws_s3_object\" \"o\" {\n  bucket = \"b\"\n  source = \"/etc/passwd\"\n}\n"},
			want: []string{"main.tf:3: aws_s3_object.o.source: an absolute path is refused"}},
		{name: "aws_lambda_function filename", files: map[string]string{"main.tf": "resource \"aws_lambda_function\" \"f\" {\n  function_name = \"f\"\n  filename      = \"../../x.zip\"\n}\n"},
			want: []string{"main.tf:3: aws_lambda_function.f.filename: a bare relative path is refused"}},
		{name: "every other aws path argument", files: map[string]string{"main.tf": `resource "aws_s3_bucket_object" "a" {
  source = "/etc/passwd"
}
resource "aws_lambda_layer_version" "a" {
  filename = "/etc/passwd"
}
resource "aws_synthetics_canary" "a" {
  zip_file = "/etc/passwd"
}
resource "aws_gamelift_script" "a" {
  zip_file = "/etc/passwd"
}
resource "aws_connect_contact_flow" "a" {
  filename = "/etc/passwd"
}
resource "aws_connect_contact_flow_module" "a" {
  filename = "/etc/passwd"
}
resource "aws_rds_custom_db_engine_version" "a" {
  filename = "/etc/passwd"
}
resource "aws_emr_cluster" "a" {
  configurations = "/etc/x.json"
}
resource "aws_dataexchange_revision_assets" "a" {
  asset {
    import_assets_from_signed_url {
      filename = "/etc/passwd"
    }
  }
}
`}, want: []string{
			"main.tf:2: aws_s3_bucket_object.a.source: an absolute path is refused",
			"main.tf:5: aws_lambda_layer_version.a.filename: an absolute path is refused",
			"main.tf:8: aws_synthetics_canary.a.zip_file: an absolute path is refused",
			"main.tf:11: aws_gamelift_script.a.zip_file: an absolute path is refused",
			"main.tf:14: aws_connect_contact_flow.a.filename: an absolute path is refused",
			"main.tf:17: aws_connect_contact_flow_module.a.filename: an absolute path is refused",
			"main.tf:20: aws_rds_custom_db_engine_version.a.filename: an absolute path is refused",
			"main.tf:23: aws_emr_cluster.a.configurations: an absolute path is refused",
			"main.tf:28: aws_dataexchange_revision_assets.a.asset.import_assets_from_signed_url.filename: an absolute path is refused",
		}},
		{name: "a nested aws path argument in a dynamic block", files: map[string]string{"main.tf": `resource "aws_dataexchange_revision_assets" "a" {
  dynamic "asset" {
    for_each = [1]
    content {
      import_assets_from_signed_url {
        filename = "/etc/passwd"
      }
    }
  }
}
`}, want: []string{"main.tf:6: aws_dataexchange_revision_assets.a.asset.import_assets_from_signed_url.filename: an absolute path is refused"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := gateFixture(t, tc.files, "")
			require.ErrorIs(t, err, errRehearsalRefuses)
			for _, want := range tc.want {
				assert.Contains(t, err.Error(), want)
			}
			for _, absent := range tc.absent {
				assert.NotContains(t, err.Error(), absent)
			}
		})
	}
}

// TestRehearsalGateWritingATemplateCaseInsensitively: on a case-insensitive
// filesystem (macOS's default), T.TPL is the template t.tpl, so writing it
// is refused. A case-sensitive filesystem has no such alias.
func TestRehearsalGateWritingATemplateCaseInsensitively(t *testing.T) {
	probe := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(probe, "t.tpl"), nil, 0o600))
	if _, err := os.Stat(filepath.Join(probe, "T.TPL")); err != nil {
		t.Skip("case-sensitive filesystem: T.TPL is another file")
	}
	err := gateFixture(t, map[string]string{
		"main.tf": "resource \"local_file\" \"t\" {\n  content  = \"x\"\n  filename = \"${path.module}/T.TPL\"\n}\noutput \"o\" {\n  value = templatefile(\"${path.module}/t.tpl\", {})\n}\n",
		"t.tpl":   "hello\n",
	}, "")
	require.ErrorIs(t, err, errRehearsalRefuses)
	assert.Contains(t, err.Error(), "main.tf:3: local_file.t.filename: refused: the apply would overwrite template t.tpl")
}

// TestRehearsalGatePathsAdmit: every path form the gate admits, in one
// copy, passes.
func TestRehearsalGatePathsAdmit(t *testing.T) {
	err := gateFixture(t, map[string]string{
		"main.tf": `resource "local_file" "out" {
  content  = file("${path.module}/files/a.txt")
  filename = "${path.module}/out.txt"
}

resource "local_file" "existing" {
  content  = "b"
  filename = "${path.module}/files/b.txt"
}

data "local_file" "in" {
  filename = "${path.root}/files/a.txt"
}

data "archive_file" "zip" {
  type        = "zip"
  source_dir  = "${path.module}/files"
  output_path = "${path.root}/build/files.zip"
}

resource "aws_s3_object" "o" {
  bucket = "b"
  key    = "k"
  source = "${path.module}/files/a.txt"
  etag   = filemd5("${path.module}/files/a.txt")
}

output "t" {
  value = templatefile("${path.root}/t.tpl", { name = "x" })
}

output "s" {
  value = fileset(path.module, "files/*.txt")
}

output "e" {
  value = fileexists("${path.module}")
}
`,
		"files/a.txt": "a\n",
		"files/b.txt": "b\n",
		"t.tpl":       "hello ${upper(name)}\n",
		"child/main.tf": `output "c" {
  value = file("${path.module}/../files/a.txt")
}
`,
	}, "")
	assert.NoError(t, err)
}
