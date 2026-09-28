package cli

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func parseAWSStack(t *testing.T, src string) *hclsyntax.Body {
	t.Helper()
	file, diags := hclsyntax.ParseConfig([]byte(src), "main.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())
	return file.Body.(*hclsyntax.Body)
}

// awsAttrProblems runs awsResourceProblems over every resource in src,
// against us-east-1 and src's own variable defaults.
func awsAttrProblems(t *testing.T, src, region string) []string {
	t.Helper()
	body := parseAWSStack(t, src)
	defaults := make(map[string]cty.Value)
	layer3VariableDefaults(body, defaults)
	problems := make([]string, 0)
	for _, block := range body.Blocks {
		if block.Type == "resource" {
			problems = append(problems, awsResourceProblems(block, "main.tf", defaults, region)...)
		}
	}
	return problems
}

func TestAWSResourceProblemsAdmitsWebStepOne(t *testing.T) {
	body, err := layer3ParseFile(filepath.Join("..", "e2e", "testdata", "aws-web-step-one"), "web-step-one.tf")
	require.NoError(t, err)
	require.Len(t, body.Blocks, 8, "every step-one type but aws_eip")
	for _, block := range body.Blocks {
		assert.Empty(t, awsResourceProblems(block, "web-step-one.tf", nil, "us-east-1"), block.Labels)
	}
	assert.Empty(t, awsMultiplicityProblems(map[string]*hclsyntax.Body{"web-step-one.tf": body}))

	assert.Empty(t, awsAttrProblems(t, `
variable "size" {
  default = "t3.small"
}
resource "aws_instance" "web" {
  instance_type = var.size
  depends_on    = [aws_internet_gateway.main]
  root_block_device {
    volume_size           = 20
    volume_type           = "gp3"
    delete_on_termination = true
  }
}
resource "aws_eip" "web" {
  domain   = "vpc"
  instance = aws_instance.web.id
}
resource "aws_subnet" "a" {
  availability_zone = "us-east-1a"
}
resource "aws_subnet" "f" {
  availability_zone = "us-east-1f"
}`, "us-east-1"))
}

func TestAWSResourceProblemsRefuses(t *testing.T) {
	unlisted := func(resourceType, attr string) string {
		return fmt.Sprintf("main.tf: %s web sets %s, which the AWS gate does not admit as an attribute", resourceType, attr)
	}
	unlistedBlock := func(resourceType, block string) string {
		return fmt.Sprintf("main.tf: %s web has nested block %s, which the AWS gate does not admit", resourceType, block)
	}
	unresolved := func(resourceType, attr string) string {
		return fmt.Sprintf("main.tf: %s web sets %s to something this check cannot resolve to a constant", resourceType, attr)
	}
	ref := func(resourceType, attr, want string) string {
		return fmt.Sprintf("main.tf: %s web must set %s to %s, a resource in this stack", resourceType, attr, want)
	}
	zone := func(value string) string {
		return fmt.Sprintf("main.tf: aws_subnet web sets availability_zone to %q; the gate admits only a standard zone of us-east-1", value)
	}
	root := func(body string) string {
		return "resource \"aws_instance\" \"web\" {\n  root_block_device {\n    " + body + "\n  }\n}"
	}
	on := func(resourceType, body string) string {
		return "resource \"" + resourceType + "\" \"web\" {\n  " + body + "\n}"
	}
	const sgList = `{
    from_port       = 80
    to_port         = 80
    protocol        = "tcp"
    cidr_blocks     = ["0.0.0.0/0"]
    security_groups = ["sg-0123"]
    prefix_list_ids = ["pl-x"]
  }`

	for name, tc := range map[string]struct{ src, want string }{
		// The provider's attribute-as-blocks forms.
		"ingress as attribute":           {on("aws_security_group", "ingress = ["+sgList+"]"), unlisted("aws_security_group", "ingress")},
		"egress as attribute":            {on("aws_security_group", "egress = ["+sgList+"]"), unlisted("aws_security_group", "egress")},
		"route as attribute":             {on("aws_route_table", `route = [{ cidr_block = "0.0.0.0/0", gateway_id = aws_internet_gateway.main.id }]`), unlisted("aws_route_table", "route")},
		"root_block_device as attribute": {on("aws_instance", `root_block_device = [{ volume_size = 8 }]`), unlisted("aws_instance", "root_block_device")},

		"root iops":                       {root("iops = 3001"), unlisted("aws_instance", "root_block_device.iops")},
		"root throughput":                 {root("throughput = 126"), unlisted("aws_instance", "root_block_device.throughput")},
		"root encrypted":                  {root("encrypted = true"), unlisted("aws_instance", "root_block_device.encrypted")},
		"root kms_key_id":                 {root(`kms_key_id = "arn:aws:kms:us-east-1:111111111111:key/x"`), unlisted("aws_instance", "root_block_device.kms_key_id")},
		"root tags":                       {root(`tags = { a = "b" }`), unlisted("aws_instance", "root_block_device.tags")},
		"root volume_size 21":             {root("volume_size = 21"), "main.tf: aws_instance web sets root_block_device.volume_size to 21; the gate caps it at 20"},
		"root volume_type gp2":            {root(`volume_type = "gp2"`), `main.tf: aws_instance web sets root_block_device.volume_type to "gp2"; the gate permits only [gp3]`},
		"root delete_on_termination off":  {root("delete_on_termination = false"), "main.tf: aws_instance web sets root_block_device.delete_on_termination to false; the gate permits only [true]"},
		"root volume_size var no default": {"variable \"v\" {}\n" + root("volume_size = var.v"), unresolved("aws_instance", "root_block_device.volume_size")},
		"two root_block_device": {on("aws_instance", "root_block_device {\n    volume_size = 8\n  }\n  root_block_device {\n    volume_size = 8\n  }"),
			"main.tf: aws_instance web has more than 1 root_block_device block; the gate admits at most 1"},

		"instance tenancy":                       {on("aws_instance", `tenancy = "dedicated"`), unlisted("aws_instance", "tenancy")},
		"instance ebs_block_device":              {on("aws_instance", "ebs_block_device {\n    device_name = \"/dev/sdb\"\n  }"), unlistedBlock("aws_instance", "ebs_block_device")},
		"instance network_interface":             {on("aws_instance", "network_interface {\n    device_index = 0\n  }"), unlistedBlock("aws_instance", "network_interface")},
		"instance iam_instance_profile":          {on("aws_instance", `iam_instance_profile = "admin"`), unlisted("aws_instance", "iam_instance_profile")},
		"instance disable_api_termination true":  {on("aws_instance", "disable_api_termination = true"), unlisted("aws_instance", "disable_api_termination")},
		"instance disable_api_termination false": {on("aws_instance", "disable_api_termination = false"), unlisted("aws_instance", "disable_api_termination")},
		"instance user_data_base64":              {on("aws_instance", `user_data_base64 = "ZWNobw=="`), unlisted("aws_instance", "user_data_base64")},
		"instance user_data_replace_on_change":   {on("aws_instance", "user_data_replace_on_change = true"), unlisted("aws_instance", "user_data_replace_on_change")},
		"instance metadata_options":              {on("aws_instance", "metadata_options {\n    http_tokens = \"required\"\n  }"), unlistedBlock("aws_instance", "metadata_options")},
		"instance_type t3.medium":                {on("aws_instance", `instance_type = "t3.medium"`), `main.tf: aws_instance web sets instance_type to "t3.medium"; the gate permits only [t3.micro t3.small]`},
		"instance_type var no default":           {"variable \"t\" {}\n" + on("aws_instance", "instance_type = var.t"), unresolved("aws_instance", "instance_type")},

		"Local Zone":          {on("aws_subnet", `availability_zone = "us-east-1-bos-1a"`), zone("us-east-1-bos-1a")},
		"Wavelength zone":     {on("aws_subnet", `availability_zone = "us-east-1-wl1-bos-wlz-1"`), zone("us-east-1-wl1-bos-wlz-1")},
		"zone id":             {on("aws_subnet", `availability_zone = "use1-az1"`), zone("use1-az1")},
		"region as zone":      {on("aws_subnet", `availability_zone = "us-east-1"`), zone("us-east-1")},
		"other region":        {on("aws_subnet", `availability_zone = "eu-west-1a"`), zone("eu-west-1a")},
		"zone var no default": {"variable \"az\" {}\n" + on("aws_subnet", "availability_zone = var.az"), unresolved("aws_subnet", "availability_zone")},

		"vpc dedicated":                   {on("aws_vpc", `instance_tenancy = "dedicated"`), `main.tf: aws_vpc web sets instance_tenancy to "dedicated"; the gate permits only [default]`},
		"vpc ipv4_ipam_pool_id":           {on("aws_vpc", `ipv4_ipam_pool_id = "ipam-pool-0123"`), unlisted("aws_vpc", "ipv4_ipam_pool_id")},
		"subnet outpost_arn":              {on("aws_subnet", `outpost_arn = "arn:aws:outposts:us-east-1:111111111111:outpost/op-0123"`), unlisted("aws_subnet", "outpost_arn")},
		"subnet customer_owned_ipv4_pool": {on("aws_subnet", `customer_owned_ipv4_pool = "ipv4pool-coip-0123"`), unlisted("aws_subnet", "customer_owned_ipv4_pool")},
		"eip address":                     {on("aws_eip", `address = "203.0.113.1"`), unlisted("aws_eip", "address")},
		"eip public_ipv4_pool":            {on("aws_eip", `public_ipv4_pool = "ipv4pool-ec2-0123"`), unlisted("aws_eip", "public_ipv4_pool")},
		"eip network_interface":           {on("aws_eip", `network_interface = "eni-0123"`), unlisted("aws_eip", "network_interface")},
		"eip domain standard":             {on("aws_eip", `domain = "standard"`), `main.tf: aws_eip web sets domain to "standard"; the gate permits only [vpc]`},
		"route nat_gateway_id":            {on("aws_route", `nat_gateway_id = "nat-0123"`), unlisted("aws_route", "nat_gateway_id")},
		"route vpc_peering_connection_id": {on("aws_route", `vpc_peering_connection_id = "pcx-0123"`), unlisted("aws_route", "vpc_peering_connection_id")},
		"route instance_id":               {on("aws_route", `instance_id = "i-0123"`), unlisted("aws_route", "instance_id")},
		"association gateway_id":          {on("aws_route_table_association", "gateway_id = aws_internet_gateway.main.id"), unlisted("aws_route_table_association", "gateway_id")},
		"ingress prefix_list_ids":         {on("aws_security_group", "ingress {\n    prefix_list_ids = [\"pl-x\"]\n  }"), unlisted("aws_security_group", "ingress.prefix_list_ids")},
		"ingress security_groups":         {on("aws_security_group", "ingress {\n    security_groups = [\"sg-0123\"]\n  }"), unlisted("aws_security_group", "ingress.security_groups")},

		"literal vpc id":        {on("aws_subnet", `vpc_id = "vpc-0123"`), ref("aws_subnet", "vpc_id", "aws_vpc.<name>.id")},
		"literal subnet id":     {on("aws_instance", `subnet_id = "subnet-0123"`), ref("aws_instance", "subnet_id", "aws_subnet.<name>.id")},
		"literal route table":   {on("aws_route", `route_table_id = "rtb-0123"`), ref("aws_route", "route_table_id", "aws_route_table.<name>.id")},
		"literal gateway":       {on("aws_route", `gateway_id = "igw-0123"`), ref("aws_route", "gateway_id", "aws_internet_gateway.<name>.id")},
		"literal group":         {on("aws_instance", `vpc_security_group_ids = ["sg-0123"]`), ref("aws_instance", "vpc_security_group_ids", "a list of aws_security_group.<name>.id")},
		"literal among refs":    {on("aws_instance", `vpc_security_group_ids = [aws_security_group.web.id, "sg-0123"]`), ref("aws_instance", "vpc_security_group_ids", "a list of aws_security_group.<name>.id")},
		"literal instance":      {on("aws_eip", `instance = "i-0123"`), ref("aws_eip", "instance", "aws_instance.<name>.id")},
		"var with a default":    {"variable \"vpc\" {\n  default = \"vpc-0123\"\n}\n" + on("aws_subnet", "vpc_id = var.vpc"), ref("aws_subnet", "vpc_id", "aws_vpc.<name>.id")},
		"reference wrong type":  {on("aws_subnet", "vpc_id = aws_subnet.public.id"), ref("aws_subnet", "vpc_id", "aws_vpc.<name>.id")},
		"ternary mentioning it": {"variable \"b\" {}\n" + on("aws_subnet", `vpc_id = var.b ? aws_vpc.main.id : "vpc-0123"`), ref("aws_subnet", "vpc_id", "aws_vpc.<name>.id")},
		"arn not id":            {on("aws_subnet", "vpc_id = aws_vpc.main.arn"), ref("aws_subnet", "vpc_id", "aws_vpc.<name>.id")},

		"aws_launch_template": {on("aws_launch_template", `image_id = "ami-0123"`), "main.tf: aws_launch_template web is a type the AWS gate has no attribute allowlist for"},
		"aws_nat_gateway":     {on("aws_nat_gateway", `subnet_id = aws_subnet.public.id`), "main.tf: aws_nat_gateway web is a type the AWS gate has no attribute allowlist for"},
	} {
		t.Run(name, func(t *testing.T) {
			problems := awsAttrProblems(t, tc.src, "us-east-1")
			require.Len(t, problems, 1, "the mutation alone is refused")
			assert.Contains(t, problems[0], tc.want)
		})
	}
}

func TestAWSResourceProblemsRefusesAZoneWithNoRegion(t *testing.T) {
	problems := awsAttrProblems(t, `resource "aws_subnet" "web" {
  availability_zone = "us-east-1a"
}`, "")
	assert.Equal(t, []string{"main.tf: aws_subnet web sets availability_zone, and no region is configured to check it against"}, problems)
}

func TestAWSMultiplicityProblems(t *testing.T) {
	const instance = "resource \"aws_instance\" \"web\" {}\n"
	const eip = "resource \"aws_eip\" \"%s\" {}\n"
	for name, tc := range map[string]struct {
		files map[string]string
		want  string
	}{
		"no instance": {map[string]string{"main.tf": `resource "aws_vpc" "main" {}`, "net.tf": `resource "aws_subnet" "public" {}`},
			"no aws_instance is declared in main.tf, net.tf"},
		"two instances in one file": {map[string]string{"main.tf": instance + `resource "aws_instance" "other" {}`},
			"aws_instance is declared 2 times (main.tf: web, main.tf: other)"},
		"two instances across files": {map[string]string{"a.tf": instance, "b.tf": instance},
			"aws_instance is declared 2 times (a.tf: web, b.tf: web)"},
		"two eips": {map[string]string{"main.tf": instance + fmt.Sprintf(eip, "one"), "extra.tf": fmt.Sprintf(eip, "two")},
			"aws_eip is declared 2 times (extra.tf: two, main.tf: one)"},
	} {
		t.Run(name, func(t *testing.T) {
			parsed := make(map[string]*hclsyntax.Body)
			for file, src := range tc.files {
				parsed[file] = parseAWSStack(t, src)
			}
			problems := awsMultiplicityProblems(parsed)
			require.Len(t, problems, 1)
			assert.Contains(t, problems[0], tc.want)
		})
	}

	assert.Empty(t, awsMultiplicityProblems(map[string]*hclsyntax.Body{"main.tf": parseAWSStack(t, instance+fmt.Sprintf(eip, "one"))}),
		"one instance and one address")
}
