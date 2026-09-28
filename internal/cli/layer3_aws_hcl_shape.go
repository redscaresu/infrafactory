package cli

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// layer3AWSResourcePrefix is the prefix of every resource type an AWS
// stack may declare.
const layer3AWSResourcePrefix = "aws_"

// awsGateInputs is what the AWS gate checks a stack against, beyond the
// stack itself. Every field is required: a zero value refuses, and never
// defaults (the gate cannot check a region, an image or a script it was
// not given).
type awsGateInputs struct {
	AllowedResourceTypes []string
	// Region is aws.region as configured, not awsRegion's default.
	Region string
	// AMI is the image this run resolved and its root mapping.
	AMI awsResolvedAMI
	// UserData is renderAWSUserData's script for this run.
	UserData []byte
}

// validateAWSLayer3HCLShape is the AWS gate's driver: every structural
// check an AWS Layer 3 stack passes before any tofu starts. Like
// validateLayer3HCLShape it only calls named rules, so each has a parity
// row against the Scaleway gate (layer3_aws_parity_test.go).
func validateAWSLayer3HCLShape(outputDir string, in awsGateInputs) error {
	parsed, varDefaults, err := layer3ParseDir(outputDir)
	if err != nil {
		return err
	}
	problems := make([]string, 0)
	for _, file := range slices.Sorted(maps.Keys(parsed)) {
		for _, block := range parsed[file].Blocks {
			problems = append(problems, awsBlockProblems(block, file, varDefaults, in)...)
		}
	}
	problems = append(problems, awsProviderProblems(parsed, varDefaults, in.Region)...)
	problems = append(problems, awsMultiplicityProblems(parsed)...)
	problems = append(problems, awsAMIRootProblems(in.AMI)...)
	if problem := awsUserDataFileProblem(outputDir, in.UserData); problem != "" {
		problems = append(problems, problem)
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%w: %s", ErrLayer3RefusesConfiguration, strings.Join(problems, "; "))
	}
	return nil
}

// awsBlockProblems runs the per-block rules, then the per-resource ones. A
// block type that is not admitted is refused alone, as the Scaleway driver
// does: its contents are not a Layer 3 stack's.
func awsBlockProblems(block *hclsyntax.Block, file string, varDefaults map[string]cty.Value, in awsGateInputs) []string {
	if problem := layer3TopLevelBlockProblem(block, file); problem != "" {
		return []string{problem}
	}
	problems := layer3NestedProblems(block, file)
	problems = append(problems, layer3FunctionCallProblems(block, file, awsUserDataExempt)...)
	problems = append(problems, layer3UndestroyableProblems(block, file)...)
	if block.Type != "resource" {
		return problems
	}
	if problem := layer3ResourceTypeProblem(block, file, layer3AWSResourcePrefix, in.AllowedResourceTypes); problem != "" {
		problems = append(problems, problem)
	}
	problems = append(problems, layer3MultiplicityProblems(block, file)...)
	problems = append(problems, awsResourceProblems(block, file, varDefaults, in.Region)...)
	if len(block.Labels) > 0 && block.Labels[0] == "aws_instance" {
		problems = append(problems, awsInstanceUserDataProblems(block, file)...)
		problems = append(problems, awsAMIProblems(block, file, in.AMI)...)
	}
	return problems
}
