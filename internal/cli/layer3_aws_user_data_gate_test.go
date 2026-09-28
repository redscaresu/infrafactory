package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

const awsUserDataExact = `file("${path.module}/infrafactory-user-data.sh")`

func parseHCLBlocks(t *testing.T, src string) []*hclsyntax.Block {
	t.Helper()
	file, diags := hclsyntax.ParseConfig([]byte(src), "main.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())
	return file.Body.(*hclsyntax.Body).Blocks
}

func awsInstanceWith(t *testing.T, body string) *hclsyntax.Block {
	t.Helper()
	return parseHCLBlocks(t, "resource \"aws_instance\" \"web\" {\n"+body+"\n}\n")[0]
}

func TestAWSUserDataExemptAdmitsTheWebStepOneLine(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "e2e", "testdata", "aws-web-step-one", "web-step-one.tf"))
	require.NoError(t, err)
	require.Contains(t, string(src), generator.AWSUserDataLine)

	var instance *hclsyntax.Block
	for _, block := range parseHCLBlocks(t, string(src)) {
		if block.Type == "resource" && block.Labels[0] == "aws_instance" {
			instance = block
		}
	}
	require.NotNil(t, instance)

	assert.True(t, awsUserDataExempt(instance, instance.Body.Attributes["user_data"]))
	assert.Empty(t, awsInstanceUserDataProblems(instance, "main.tf"))
	assert.Empty(t, layer3FunctionCallProblems(instance, "main.tf", awsUserDataExempt))
}

func TestAWSUserDataRefusesEveryOtherShape(t *testing.T) {
	for name, expr := range map[string]string{
		"relative path":         `file("infrafactory-user-data.sh")`,
		"proc environ":          `file("/proc/self/environ")`,
		"path.root":             `file("${path.root}/infrafactory-user-data.sh")`,
		"path.cwd":              `file("${path.cwd}/infrafactory-user-data.sh")`,
		"parent directory":      `file("${path.module}/../infrafactory-user-data.sh")`,
		"trailing space":        `file("${path.module}/infrafactory-user-data.sh ")`,
		"local path":            `file(local.p)`,
		"expanded args":         `file(local.args...)`,
		"expanded exact arg":    `file("${path.module}/infrafactory-user-data.sh"...)`,
		"path.module.x":         `file("${path.module.x}/infrafactory-user-data.sh")`,
		"local.module":          `file("${local.module}/infrafactory-user-data.sh")`,
		"second argument":       `file("${path.module}/infrafactory-user-data.sh", "x")`,
		"interpolated suffix":   `file("${path.module}/infrafactory-user-data.sh${local.s}")`,
		"core::file":            `core::file("${path.module}/infrafactory-user-data.sh")`,
		"filebase64":            `filebase64("${path.module}/infrafactory-user-data.sh")`,
		"templatefile":          `templatefile("${path.module}/infrafactory-user-data.sh", {})`,
		"wrapped in trimspace":  `trimspace(` + awsUserDataExact + `)`,
		"interpolated":          `"${` + awsUserDataExact + `}"`,
		"concatenated with env": `"${` + awsUserDataExact + `}${file("/proc/self/environ")}"`,
		"heredoc argument":      "file(<<EOT\n${path.module}/infrafactory-user-data.sh\nEOT\n)",
		"heredoc":               "<<EOT\n#!/bin/bash\nEOT\n",
		"literal":               `"#!/bin/bash"`,
		"variable":              `var.ud`,
		"local":                 `local.ud`,
	} {
		t.Run(name, func(t *testing.T) {
			block := awsInstanceWith(t, "user_data = "+expr)
			assert.False(t, awsUserDataExempt(block, block.Body.Attributes["user_data"]))
			assert.Len(t, awsInstanceUserDataProblems(block, "main.tf"), 1)
		})
	}

	problems := awsInstanceUserDataProblems(awsInstanceWith(t, `ami = "ami-0abc"`), "main.tf")
	assert.Equal(t, []string{"main.tf: aws_instance web must set exactly `" + generator.AWSUserDataLine +
		"`; the instance boots only the script infrafactory renders"}, problems, "user_data absent")
}

// The exact expression is exempt only as user_data directly on an
// aws_instance. Anywhere else it is a file() call like any other.
func TestAWSUserDataExemptIsOnlyUserDataOnAnInstance(t *testing.T) {
	for name, src := range map[string]string{
		"instance tags": "resource \"aws_instance\" \"web\" {\n  tags = " + awsUserDataExact + "\n}",
		"output":        "output \"ud\" {\n  value = " + awsUserDataExact + "\n}",
		"local":         "locals {\n  ud = " + awsUserDataExact + "\n}",
		"aws_eip":       "resource \"aws_eip\" \"ip\" {\n  user_data = " + awsUserDataExact + "\n}",
		"data source":   "data \"aws_instance\" \"web\" {\n  user_data = " + awsUserDataExact + "\n}",
	} {
		t.Run(name, func(t *testing.T) {
			block := parseHCLBlocks(t, src)[0]
			require.Len(t, block.Body.Attributes, 1)
			for _, attr := range block.Body.Attributes {
				assert.False(t, awsUserDataExempt(block, attr))
				assert.Equal(t, []string{attr.Name}, callers(layer3FunctionCallProblems(block, "main.tf", awsUserDataExempt)))
			}
		})
	}
}

func TestAWSAMIProblems(t *testing.T) {
	resolved := awsResolvedAMI{ID: "ami-0abc"}
	assert.Empty(t, awsAMIProblems(awsInstanceWith(t, `ami = "ami-0abc"`), "main.tf", resolved))

	for name, tc := range map[string]struct {
		body     string
		resolved awsResolvedAMI
	}{
		"another id":     {`ami = "ami-0def"`, resolved},
		"variable":       {`ami = var.ami`, resolved},
		"empty resolved": {`ami = ""`, awsResolvedAMI{}},
		"missing":        {`instance_type = "t3.micro"`, resolved},
	} {
		assert.Len(t, awsAMIProblems(awsInstanceWith(t, tc.body), "main.tf", tc.resolved), 1, name)
	}
}

func TestAWSAMIRootProblems(t *testing.T) {
	for _, tc := range []struct {
		root  harness.AWSAMIRoot
		admit bool
	}{
		{harness.AWSAMIRoot{SizeGiB: 8, VolumeType: "gp3", DeleteOnTermination: true}, true},
		{harness.AWSAMIRoot{SizeGiB: 20, VolumeType: "gp3", DeleteOnTermination: true}, true},
		{harness.AWSAMIRoot{SizeGiB: 21, VolumeType: "gp3", DeleteOnTermination: true}, false},
		{harness.AWSAMIRoot{SizeGiB: 8, VolumeType: "gp2", DeleteOnTermination: true}, false},
		{harness.AWSAMIRoot{SizeGiB: 8, VolumeType: "gp3", DeleteOnTermination: false}, false},
		{harness.AWSAMIRoot{SizeGiB: 0, VolumeType: "gp3", DeleteOnTermination: true}, false},
		{harness.AWSAMIRoot{}, false},
	} {
		problems := awsAMIRootProblems(awsResolvedAMI{ID: "ami-0abc", Root: tc.root})
		assert.Equal(t, tc.admit, len(problems) == 0, "%+v: %v", tc.root, problems)
	}
}

func TestAWSUserDataFileProblem(t *testing.T) {
	rendered, err := renderAWSUserData(scenario.ServiceSpec{Image: "nginx", Tag: "1.27", Port: 80})
	require.NoError(t, err)

	write := func(t *testing.T, data []byte) string {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, generator.AWSUserDataFile), data, 0o600))
		return dir
	}
	assert.Empty(t, awsUserDataFileProblem(write(t, rendered), rendered))

	differs := append([]byte{}, rendered...)
	differs[len(differs)/2] ^= 1

	for name, tc := range map[string]struct {
		dir      func(t *testing.T) string
		rendered []byte
	}{
		"missing": {func(t *testing.T) string { return t.TempDir() }, rendered},
		"directory": {func(t *testing.T) string {
			dir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dir, generator.AWSUserDataFile), 0o700))
			return dir
		}, rendered},
		"symlink to an equal file": {func(t *testing.T) string {
			dir := t.TempDir()
			target := filepath.Join(dir, "elsewhere.sh")
			require.NoError(t, os.WriteFile(target, rendered, 0o600))
			require.NoError(t, os.Symlink(target, filepath.Join(dir, generator.AWSUserDataFile)))
			return dir
		}, rendered},
		"one byte differs":         {func(t *testing.T) string { return write(t, differs) }, rendered},
		"no trailing newline":      {func(t *testing.T) string { return write(t, rendered[:len(rendered)-1]) }, rendered},
		"nothing rendered":         {func(t *testing.T) string { return write(t, rendered) }, nil},
		"empty file, empty render": {func(t *testing.T) string { return write(t, nil) }, []byte{}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, awsUserDataFileProblem(tc.dir(t), tc.rendered), generator.AWSUserDataFile+": ")
		})
	}
}

// The AWS exception is AWS's alone: the Scaleway driver passes no
// predicate, so the same expression on a Scaleway server is refused.
func TestLayer3ScalewayStackRefusesTheAWSUserDataExpression(t *testing.T) {
	allowlist := append(append([]string{}, gateAllowlist...), "scaleway_instance_server")
	err := validateLayer3HCLShape(writeShapeHCL(t, shapeProject+`
resource "scaleway_instance_server" "web" {
  type      = "DEV1-S"
  image     = "ubuntu_jammy"
  user_data = `+awsUserDataExact+`
}`), allowlist)
	require.ErrorIs(t, err, ErrLayer3RefusesConfiguration)
	assert.Contains(t, err.Error(), "main.tf: user_data calls file(), which is not on the pure-function allowlist")
}

// Why awsAMIRootProblems bounds the image rather than requiring
// root_block_device: fakeaws applies web-step-one with none.
func TestFakeAWSStateCarriesNoRootBlockDevice(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "harness", "testdata", "realprobe", "aws", "terraform-live.tfstate"))
	require.NoError(t, err)
	var state struct {
		Resources []struct {
			Type      string
			Instances []struct {
				Attributes struct {
					RootBlockDevice []any `json:"root_block_device"`
				}
			}
		}
	}
	require.NoError(t, json.Unmarshal(src, &state))
	instances := 0
	for _, resource := range state.Resources {
		if resource.Type != "aws_instance" {
			continue
		}
		for _, instance := range resource.Instances {
			instances++
			assert.Empty(t, instance.Attributes.RootBlockDevice)
		}
	}
	assert.Equal(t, 1, instances)
}
