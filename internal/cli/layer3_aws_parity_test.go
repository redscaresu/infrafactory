package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// S156d: a new path inherits the rules the old one enforced only if each
// is asked again. Every rule and table entry of the Scaleway gate is a
// source, and each source has a row saying what the AWS gate does about it.

// awsParityRow is applied, when edit is set: that mutation of the admitted
// stack is refused by validateAWSLayer3HCLShape with want in the refusal.
// Otherwise it is declined, and declined says why AWS needs no such rule.
type awsParityRow struct {
	edit     awsStackEdit
	want     string
	declined string
}

// tofuTopLevelBlocks is every top-level block OpenTofu 1.12 accepts.
var tofuTopLevelBlocks = []string{
	"terraform", "provider", "variable", "output", "locals", "resource",
	"data", "module", "import", "moved", "removed", "check", "ephemeral",
}

// awsParitySources is what the sources are read from, so the checker can
// be shown a source it has no row for.
type awsParitySources struct {
	shapeGo      []byte // layer3_hcl_shape.go
	deniedNested map[string]bool
	numeric      map[string]map[string]float64
	enum         map[string]map[string][]string
	nestedCost   map[string]float64
}

func scalewayGateSources(t *testing.T) awsParitySources {
	t.Helper()
	src, err := os.ReadFile("layer3_hcl_shape.go")
	require.NoError(t, err)
	return awsParitySources{
		shapeGo:      src,
		deniedNested: layer3DeniedNestedBlocks,
		numeric:      layer3NumericBounds,
		enum:         layer3EnumBounds,
		nestedCost:   layer3NestedCostBounds,
	}
}

// keys names every source: each func whose name ends in Problem or
// Problems, the parse step, each denied nested block, each top-level block
// tofu accepts and the gate does not, and each bounds entry.
func (s awsParitySources) keys(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "layer3_hcl_shape.go", s.shapeGo, 0)
	require.NoError(t, err)
	keys := []string{"layer3UnreadableConfigExt", "layer3ParseDir"}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && (strings.HasSuffix(fn.Name.Name, "Problem") || strings.HasSuffix(fn.Name.Name, "Problems")) {
			keys = append(keys, fn.Name.Name)
		}
	}
	for block := range s.deniedNested {
		keys = append(keys, "layer3DeniedNestedBlocks["+block+"]")
	}
	for _, block := range tofuTopLevelBlocks {
		if !layer3AllowedTopLevelBlocks[block] {
			keys = append(keys, "top-level "+block)
		}
	}
	for resourceType, attrs := range s.numeric {
		for attr := range attrs {
			keys = append(keys, "layer3NumericBounds["+resourceType+"."+attr+"]")
		}
	}
	for resourceType, attrs := range s.enum {
		for attr := range attrs {
			keys = append(keys, "layer3EnumBounds["+resourceType+"."+attr+"]")
		}
	}
	for attr := range s.nestedCost {
		keys = append(keys, "layer3NestedCostBounds["+attr+"]")
	}
	slices.Sort(keys)
	return keys
}

// awsParityGaps is every way rows fail to answer sources, one line each.
func awsParityGaps(t *testing.T, sources []string, rows map[string]awsParityRow) []string {
	t.Helper()
	gaps := make([]string, 0)
	for _, source := range sources {
		row, ok := rows[source]
		switch {
		case !ok:
			gaps = append(gaps, source+": no row")
		case row.edit == nil && strings.TrimSpace(row.declined) == "":
			gaps = append(gaps, source+": declined with no reason")
		case row.edit != nil && (row.declined != "" || row.want == ""):
			gaps = append(gaps, source+": an applied row must carry a want and no reason")
		case row.edit != nil:
			if gap := awsAppliedRowGap(t, row); gap != "" {
				gaps = append(gaps, source+": "+gap)
			}
		}
	}
	for _, source := range slices.Sorted(maps.Keys(rows)) {
		if !slices.Contains(sources, source) {
			gaps = append(gaps, source+": a row for no source")
		}
	}
	return gaps
}

func awsAppliedRowGap(t *testing.T, row awsParityRow) string {
	t.Helper()
	stack := awsAdmittedStack(t)
	row.edit(stack)
	err := awsGateOn(t, stack, awsAdmittedInputs(t))
	if err == nil {
		return "the mutation is admitted"
	}
	if !strings.Contains(err.Error(), row.want) {
		return fmt.Sprintf("refused without %q: %v", row.want, err)
	}
	return ""
}

var awsRootVolume21 = awsInside("aws_instance", "web", "  root_block_device {\n    volume_size = 21\n  }")

func awsExtraFile(content string) awsStackEdit { return awsWithFile("extra.tf", content) }

func awsTopLevel(block, content string) awsParityRow {
	return awsParityRow{edit: awsExtraFile(content), want: `extra.tf: "` + block + `" blocks are not permitted`}
}

func awsDeniedNested(block string, edit awsStackEdit, file string) awsParityRow {
	return awsParityRow{edit: edit, want: file + `: "` + block + `" executes commands during apply`}
}

// awsParityRows answers every source for the AWS gate.
var awsParityRows = map[string]awsParityRow{
	// The functions of layer3_hcl_shape.go.
	"layer3BlockProblems": {declined: "Scaleway's per-block driver, not a rule: it only dispatches to named rules, each of which has its own row; awsBlockProblems is AWS's"},
	"layer3TopLevelBlockProblem": {edit: awsExtraFile("data \"aws_ami\" \"al2023\" {\n  most_recent = true\n}\n"),
		want: `extra.tf: "data" blocks are not permitted`},
	"layer3ResourceTypeProblem": {edit: awsExtraFile("resource \"aws_launch_template\" \"web\" {\n  image_id = \"ami-0al2023x8664\"\n}\n"),
		want: `extra.tf: resource type "aws_launch_template" is not in allow_resource_types`},
	"layer3ProjectCountProblem": {declined: "the AWS scope is the account, not a project a stack could declare: the credential bounds it, and UnplacedAWSResources checks every resource against it after apply"},
	"layer3ContainmentProblems": {declined: "its project_id rule places a resource in the run's Scaleway project; the AWS scope is the account, which the credential bounds and UnplacedAWSResources checks after apply. Its parent-binding half is layer3ParentBindingProblems's row"},
	"layer3ParentBindingProblems": {edit: awsReplacing("main.tf", "vpc_id = aws_vpc.main.id", `vpc_id = "vpc-0123"`),
		want: "main.tf: aws_internet_gateway main must set vpc_id to aws_vpc.<name>.id, a resource in this stack"},
	"layer3MissingProviderProblem": {edit: awsWithFile("providers.tf", buildAwsProviderBlock("us-east-1", awsGateTestRunID)),
		want: `no terraform.required_providers entry declares source "hashicorp/aws"`},
	"layer3ProviderBlockProblems": {edit: awsReplacing("providers.tf", "s3_use_path_style = true", "s3_use_path_style = true\n  profile = \"prod\""),
		want: `providers.tf: provider setting "profile" is not permitted`},
	"layer3TerraformBlockProblems": {edit: awsExtraFile("terraform {\n  encryption {\n    key_provider \"external\" \"k\" {\n      command = [\"sh\", \"-c\", \"id\"]\n    }\n  }\n}\n"),
		want: `extra.tf: terraform block "encryption" is not permitted in a Layer 3 stack`},
	"layer3ProviderSourceProblems": {edit: awsReplacing("providers.tf", `"hashicorp/aws"`, `"attacker/aws"`),
		want: `providers.tf: required_provider "aws" must be source "hashicorp/aws"`},
	"layer3ProviderVersionProblem": {edit: awsReplacing("providers.tf", `"`+harness.AWSProviderVersion+`"`, `"5.99.0"`),
		want: `providers.tf: required_provider "aws" pins version "5.99.0"`},
	"layer3NestedProblems": {edit: awsInside("aws_instance", "web", "  provisioner \"local-exec\" {\n    command = \"id\"\n  }"),
		want: `main.tf: "provisioner" executes commands during apply`},
	"layer3FunctionCallProblems": {edit: awsInside("aws_vpc", "main", `  tags = { leak = file("/proc/self/environ") }`),
		want: "main.tf: tags calls file(), which is not on the pure-function allowlist"},
	"layer3MultiplicityProblems": {edit: awsInside("aws_instance", "web", "  count = 2"),
		want: "main.tf: web sets count"},
	"layer3CostProblems": {edit: awsReplacing("main.tf", `"t3.micro"`, `"t3.medium"`),
		want: `main.tf: aws_instance web sets instance_type to "t3.medium"; the gate permits only [t3.micro t3.small]`},
	"layer3NestedCostProblems": {edit: awsRootVolume21,
		want: "main.tf: aws_instance web sets root_block_device.volume_size to 21; the gate caps it at 20"},
	"layer3UndestroyableProblems": {edit: awsExtraFile("output \"ip\" {\n  value = aws_instance.web.ipv6_addresses[0]\n}\n"),
		want: "extra.tf: value indexes aws_instance.web.ipv6_addresses"},
	"layer3UndestroyableResourceProblems": {declined: "a Scaleway standardisation (attachments are declared on the server, not as a standalone NIC); AWS's standalone NIC, aws_network_interface, is not allowlisted, so layer3ResourceTypeProblem refuses it"},
	"layer3InlinePrivateNetworkProblems":  {declined: "AWS attaches an instance through subnet_id and vpc_security_group_ids, which awsResourceProblems requires to be typed references to resources in this stack; there is no private_network block to contain"},

	// Parsing.
	"layer3UnreadableConfigExt": {edit: awsWithFile("terraform.tfvars", "size = 21\n"), want: "layer 3 refuses terraform.tfvars: tofu loads it automatically"},
	"layer3ParseDir":            {edit: awsWithFile("broken.tf", "resource \"aws_vpc\" {\n"), want: "layer 3 refuses broken.tf: cannot parse it"},

	// layer3DeniedNestedBlocks.
	"layer3DeniedNestedBlocks[provisioner]": awsDeniedNested("provisioner", awsInside("aws_instance", "web", "  provisioner \"local-exec\" {\n    command = \"id\"\n  }"), "main.tf"),
	"layer3DeniedNestedBlocks[connection]":  awsDeniedNested("connection", awsInside("aws_instance", "web", "  connection {\n    host = \"203.0.113.1\"\n  }"), "main.tf"),
	"layer3DeniedNestedBlocks[lifecycle]":   awsDeniedNested("lifecycle", awsInside("aws_vpc", "main", "  lifecycle {\n    prevent_destroy = false\n  }"), "main.tf"),
	"layer3DeniedNestedBlocks[backend]":     awsDeniedNested("backend", awsExtraFile("terraform {\n  backend \"local\" {\n  }\n}\n"), "extra.tf"),
	"layer3DeniedNestedBlocks[cloud]":       awsDeniedNested("cloud", awsExtraFile("terraform {\n  cloud {\n    organization = \"elsewhere\"\n  }\n}\n"), "extra.tf"),
	"layer3DeniedNestedBlocks[dynamic]": awsDeniedNested("dynamic", awsReplacing("main.tf", awsSecurityGroupIngress,
		"  dynamic \"ingress\" {\n    for_each = [80]\n    content {\n      from_port   = ingress.value\n      to_port     = ingress.value\n      protocol    = \"tcp\"\n      cidr_blocks = [\"0.0.0.0/0\"]\n    }\n  }"), "main.tf"),

	// Top-level blocks tofu accepts and the gate does not.
	"top-level data":      awsTopLevel("data", "data \"aws_ami\" \"al2023\" {\n  most_recent = true\n}\n"),
	"top-level module":    awsTopLevel("module", "module \"web\" {\n  source = \"./web\"\n}\n"),
	"top-level import":    awsTopLevel("import", "import {\n  to = aws_vpc.main\n  id = \"vpc-0123\"\n}\n"),
	"top-level moved":     awsTopLevel("moved", "moved {\n  from = aws_vpc.old\n  to   = aws_vpc.main\n}\n"),
	"top-level removed":   awsTopLevel("removed", "removed {\n  from = aws_vpc.old\n}\n"),
	"top-level check":     awsTopLevel("check", "check \"up\" {\n  assert {\n    condition     = true\n    error_message = \"down\"\n  }\n}\n"),
	"top-level ephemeral": awsTopLevel("ephemeral", "ephemeral \"aws_secretsmanager_secret_version\" \"s\" {\n  secret_id = \"x\"\n}\n"),

	// Bounds. A standalone Scaleway volume's AWS analogue, aws_ebs_volume,
	// is refused outright, at any size.
	"layer3NumericBounds[scaleway_block_volume.size_in_gb]": {edit: awsExtraFile("resource \"aws_ebs_volume\" \"data\" {\n  availability_zone = \"us-east-1a\"\n  size              = 21\n}\n"),
		want: `extra.tf: resource type "aws_ebs_volume" is not in allow_resource_types`},
	"layer3NumericBounds[scaleway_block_volume.iops]": {edit: awsExtraFile("resource \"aws_ebs_volume\" \"data\" {\n  availability_zone = \"us-east-1a\"\n  iops              = 16000\n}\n"),
		want: `extra.tf: resource type "aws_ebs_volume" is not in allow_resource_types`},
	"layer3EnumBounds[scaleway_instance_server.type]": {edit: awsReplacing("main.tf", `"t3.micro"`, `"t3.medium"`),
		want: `main.tf: aws_instance web sets instance_type to "t3.medium"; the gate permits only [t3.micro t3.small]`},
	"layer3EnumBounds[scaleway_lb.type]": {declined: "aws_lb is not allowlisted at step one, so there is no load balancer type to bound; layer3ResourceTypeProblem refuses one"},
	"layer3NestedCostBounds[size_in_gb]": {edit: awsRootVolume21,
		want: "main.tf: aws_instance web sets root_block_device.volume_size to 21; the gate caps it at 20"},
	"layer3NestedCostBounds[iops]": {edit: awsInside("aws_instance", "web", "  root_block_device {\n    iops = 3001\n  }"),
		want: "main.tf: aws_instance web sets root_block_device.iops, which the AWS gate does not admit as an attribute"},
}

func TestAWSGateAnswersEveryScalewayRule(t *testing.T) {
	sources := scalewayGateSources(t)
	assert.Empty(t, awsParityGaps(t, sources.keys(t), awsParityRows))
}

// The checker has to fail on each way a row can be missing or wrong, not
// only pass on the real table.
func TestAWSParityCheckerFails(t *testing.T) {
	gapsWith := func(t *testing.T, sources awsParitySources, rows map[string]awsParityRow) []string {
		return awsParityGaps(t, sources.keys(t), rows)
	}

	t.Run("a func with no row", func(t *testing.T) {
		sources := scalewayGateSources(t)
		sources.shapeGo = append(sources.shapeGo, "\nfunc layer3NewRuleProblems() []string { return nil }\n"...)
		assert.Equal(t, []string{"layer3NewRuleProblems: no row"}, gapsWith(t, sources, awsParityRows))
	})

	t.Run("a denied nested block with no row", func(t *testing.T) {
		sources := scalewayGateSources(t)
		sources.deniedNested = maps.Clone(sources.deniedNested)
		sources.deniedNested["new_block"] = true
		assert.Equal(t, []string{"layer3DeniedNestedBlocks[new_block]: no row"}, gapsWith(t, sources, awsParityRows))
	})

	t.Run("bounds entries with no row", func(t *testing.T) {
		sources := scalewayGateSources(t)
		sources.numeric = maps.Clone(sources.numeric)
		sources.numeric["scaleway_new"] = map[string]float64{"size": 1}
		sources.enum = maps.Clone(sources.enum)
		sources.enum["scaleway_new"] = map[string][]string{"type": {"S"}}
		sources.nestedCost = maps.Clone(sources.nestedCost)
		sources.nestedCost["new_size"] = 1
		assert.Equal(t, []string{
			"layer3EnumBounds[scaleway_new.type]: no row",
			"layer3NestedCostBounds[new_size]: no row",
			"layer3NumericBounds[scaleway_new.size]: no row",
		}, gapsWith(t, sources, awsParityRows))
	})

	t.Run("an applied row whose mutation is admitted", func(t *testing.T) {
		rows := maps.Clone(awsParityRows)
		rows["layer3CostProblems"] = awsParityRow{edit: awsReplacing("main.tf", `"t3.micro"`, `"t3.small"`), want: "t3.small"}
		assert.Equal(t, []string{"layer3CostProblems: the mutation is admitted"}, gapsWith(t, scalewayGateSources(t), rows))
	})

	t.Run("an applied row refused for another reason", func(t *testing.T) {
		rows := maps.Clone(awsParityRows)
		rows["layer3CostProblems"] = awsParityRow{edit: awsWithFile("broken.tf", "{"), want: "t3.medium"}
		gaps := gapsWith(t, scalewayGateSources(t), rows)
		require.Len(t, gaps, 1)
		assert.Contains(t, gaps[0], `layer3CostProblems: refused without "t3.medium"`)
	})

	t.Run("a declined row with no reason", func(t *testing.T) {
		rows := maps.Clone(awsParityRows)
		rows["layer3ProjectCountProblem"] = awsParityRow{declined: " "}
		assert.Equal(t, []string{"layer3ProjectCountProblem: declined with no reason"}, gapsWith(t, scalewayGateSources(t), rows))
	})

	t.Run("a row for no source", func(t *testing.T) {
		rows := maps.Clone(awsParityRows)
		rows["layer3GoneProblem"] = awsParityRow{declined: "was removed"}
		assert.Equal(t, []string{"layer3GoneProblem: a row for no source"}, gapsWith(t, scalewayGateSources(t), rows))
	})
}

// The attribute table and allow_resource_types name the same AWS types:
// a type on the list with no table entry is refused by awsResourceProblems
// whatever it sets, and one in the table but not on the list is never
// reached.
func awsTableAllowlistDiff(table map[string]map[string]awsRule, allowlist []string) []string {
	diff := make([]string, 0)
	for _, entry := range allowlist {
		if _, ok := table[entry]; strings.HasPrefix(entry, layer3AWSResourcePrefix) && !ok {
			diff = append(diff, entry+" is allowlisted with no attribute table entry")
		}
	}
	for _, resourceType := range slices.Sorted(maps.Keys(table)) {
		if !slices.Contains(allowlist, resourceType) {
			diff = append(diff, resourceType+" has an attribute table entry and is not allowlisted")
		}
	}
	return diff
}

func TestAWSAttributeTableMatchesTheAllowlist(t *testing.T) {
	allowlist := defaultSandboxAllowlistForTest(t)
	assert.Empty(t, awsTableAllowlistDiff(awsAttrAllowlist, allowlist))

	assert.Equal(t, []string{"aws_nat_gateway is allowlisted with no attribute table entry"},
		awsTableAllowlistDiff(awsAttrAllowlist, append(slices.Clone(allowlist), "aws_nat_gateway")))

	table := maps.Clone(awsAttrAllowlist)
	table["aws_nat_gateway"] = map[string]awsRule{}
	assert.Equal(t, []string{"aws_nat_gateway has an attribute table entry and is not allowlisted"},
		awsTableAllowlistDiff(table, allowlist))
}
