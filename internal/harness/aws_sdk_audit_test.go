package harness

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const awsSDKConfigModule = "github.com/aws/aws-sdk-go-v2/config"

// awsDefaultChainUses names each way a file reaches the AWS SDK default
// chain: an import of the config module, or any selector named
// LoadDefaultConfig. NewAWSSTSClient's godoc says why neither may appear
// outside tests.
func awsDefaultChainUses(file *ast.File) []string {
	var uses []string
	for _, imp := range file.Imports {
		if path, _ := strconv.Unquote(imp.Path.Value); path == awsSDKConfigModule {
			uses = append(uses, "imports "+awsSDKConfigModule)
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "LoadDefaultConfig" {
			uses = append(uses, "uses a selector named LoadDefaultConfig")
		}
		return true
	})
	return uses
}

func TestNoProductionCodeUsesTheAWSDefaultChain(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	var violations []string
	parsed := 0
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "testdata" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			parsed++
			rel, _ := filepath.Rel(root, path)
			for _, use := range awsDefaultChainUses(file) {
				violations = append(violations, rel+": "+use)
			}
			return nil
		})
		require.NoError(t, err)
	}
	require.NotZero(t, parsed, "the audit parsed no files; it is looking in the wrong place")
	assert.Empty(t, violations, "production code must build AWS clients with NewAWSSTSClient-style static config, never the SDK default chain")
}

func TestAWSDefaultChainAuditFixtures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		want []string
	}{
		{"config import", `package p
import awscfg "github.com/aws/aws-sdk-go-v2/config"
var _ = awscfg.WithRegion`, []string{"imports " + awsSDKConfigModule}},
		{"LoadDefaultConfig selector", `package p
func f(x any) { x.LoadDefaultConfig(nil) }`, []string{"uses a selector named LoadDefaultConfig"}},
		{"godoc mention", `package p
// f never calls config.LoadDefaultConfig, which reads AWS_PROFILE.
func f() {}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", tc.src, parser.ParseComments)
			require.NoError(t, err)
			assert.Equal(t, tc.want, awsDefaultChainUses(file))
		})
	}
}
