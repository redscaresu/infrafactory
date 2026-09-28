package harness

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/storage"
	"github.com/open-policy-agent/opa/v1/tester"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegoPolicyTests runs every *_test.rego under policies/ and
// mutation-checks each policy that has one (ADR-0028).
func TestRegoPolicyTests(t *testing.T) {
	t.Parallel()
	root := filepath.Join(opaTestRepoRoot(t), "policies")
	loaded, problems := regoTestProblems(t.Context(), root)

	// A walk that loads nothing runs no tests and passes.
	for _, want := range []string{"aws/region_restriction.rego", "scaleway/region_restriction.rego"} {
		assert.Contains(t, loaded, filepath.Join(root, want))
	}
	for _, problem := range problems {
		t.Errorf("%s", problem)
	}
}

func TestRegoTestProblems_FailingTest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join(opaTestRepoRoot(t), "policies", "aws"))))
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join("testdata", "regotest", "failing"))))

	_, problems := regoTestProblems(t.Context(), dir)

	assert.Contains(t, problems,
		filepath.Join(dir, "public_db_passes_test.rego")+":17: data.regotest.failing_test.test_public_db_passes fails")
}

func TestRegoTestProblems_Fixtures(t *testing.T) {
	t.Parallel()
	fixture := func(name string) string { return filepath.Join("testdata", "regotest", name) }
	cases := []struct {
		dir  string
		want []string
	}{
		{fixture("killed"), nil},
		{fixture("unkilled"), []string{
			filepath.Join(fixture("unkilled"), "policy.rego") + ":14: no test fails when this deny_state body is removed",
		}},
		{fixture("notests"), []string{
			filepath.Join(fixture("notests"), "policy_test.rego") + ": no test_ rule, so it tests nothing",
		}},
		{fixture("untested"), []string{
			filepath.Join(fixture("untested"), "aws", "policy.rego") + ": no policy_test.rego beside it, and policies here require one",
		}},
	}
	for _, tc := range cases {
		t.Run(filepath.Base(tc.dir), func(t *testing.T) {
			t.Parallel()
			_, problems := regoTestProblems(t.Context(), tc.dir)
			assert.Equal(t, tc.want, problems)
		})
	}
}

// regoTestProblems loads every .rego under root, runs its tests through
// opa/v1/tester, and returns the files it loaded and every problem:
//   - a test that fails or errors;
//   - a *_test.rego with no test_ rule, which checks nothing;
//   - a policy under one of regoTestRequiredDirs with no sibling
//     <name>_test.rego;
//   - a deny or deny_state body that no test kills, in a policy with a
//     sibling <name>_test.rego.
func regoTestProblems(ctx context.Context, root string) (loaded, problems []string) {
	modules, store, err := tester.Load([]string{root}, nil)
	if err != nil {
		return nil, []string{fmt.Sprintf("load %s: %v", root, err)}
	}
	loaded = slices.Sorted(maps.Keys(modules))
	results, err := runRegoTests(ctx, modules, store)
	if err != nil {
		return loaded, []string{fmt.Sprintf("run tests under %s: %v", root, err)}
	}

	for _, r := range results {
		switch {
		case r.Error != nil:
			problems = append(problems, fmt.Sprintf("%s: %s errors: %v", r.Location, regoTestName(r), r.Error))
		case r.Fail:
			problems = append(problems, fmt.Sprintf("%s: %s fails", r.Location, regoTestName(r)))
		}
	}
	for _, file := range loaded {
		if strings.HasSuffix(file, "_test.rego") && !hasRegoTestRule(modules[file]) {
			problems = append(problems, fmt.Sprintf("%s: no %s rule, so it tests nothing", file, tester.TestPrefix))
		}
		if regoTestRequired(root, file) && modules[regoTestFile(file)] == nil {
			problems = append(problems, fmt.Sprintf("%s: no %s beside it, and policies here require one",
				file, filepath.Base(regoTestFile(file))))
		}
	}
	problems = append(problems, unkilledDenyBodies(ctx, modules, store)...)
	slices.Sort(problems)
	return loaded, problems
}

// unkilledDenyBodies makes each deny/deny_state body unsatisfiable in turn
// and reports the ones after which no test fails.
//
// The body is falsified, not deleted, so the rule stays defined. Deleting
// a package's only deny leaves `deny` undefined, and any test that counts
// it, even one asserting nothing is denied, would then kill the mutant
// without exercising the body (the unkilled fixture's
// test_bucket_in_region_passes).
func unkilledDenyBodies(ctx context.Context, modules map[string]*ast.Module, store storage.Store) []string {
	var problems []string
	for file, module := range modules {
		if strings.HasSuffix(file, "_test.rego") || modules[regoTestFile(file)] == nil {
			continue
		}
		for i, rule := range module.Rules {
			name := rule.Head.Ref().String()
			if name != "deny" && name != "deny_state" {
				continue
			}
			mutant := maps.Clone(modules)
			mutant[file] = falsifyRuleBody(module, i)
			results, err := runRegoTests(ctx, mutant, store)
			switch {
			case err != nil:
				problems = append(problems, fmt.Sprintf("%s: %s mutant does not compile: %v", rule.Location, name, err))
			case !regoMutantKilled(results):
				problems = append(problems, fmt.Sprintf("%s: no test fails when this %s body is removed", rule.Location, name))
			}
		}
	}
	return problems
}

// regoTestRequiredDirs are the directories under the policies root whose
// every policy must have a sibling _test.rego. GCP and Genesys are out of
// scope (docs/epics/policy-correctness.md).
var regoTestRequiredDirs = []string{"aws", "common", "scaleway"}

func regoTestRequired(root, file string) bool {
	rel, err := filepath.Rel(root, file)
	if err != nil || strings.HasSuffix(file, "_test.rego") {
		return false
	}
	top, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	return slices.Contains(regoTestRequiredDirs, top)
}

func regoTestFile(policy string) string {
	return strings.TrimSuffix(policy, ".rego") + "_test.rego"
}

// falsifyRuleBody returns a copy of module whose i-th rule body starts
// with `false`. Every variable stays bound by the rest of the body, so the
// mutant compiles wherever the original does.
func falsifyRuleBody(module *ast.Module, i int) *ast.Module {
	mutant := module.Copy()
	rule := mutant.Rules[i]
	rule.Body = ast.NewBody(append([]*ast.Expr{ast.NewExpr(ast.BooleanTerm(false))}, rule.Body...)...)
	return mutant
}

// regoMutantKilled reports whether any test failed or errored. A test
// already failing before the mutation kills every mutant, which hides
// unkilled bodies only while the suite is red anyway.
func regoMutantKilled(results []*tester.Result) bool {
	return slices.ContainsFunc(results, func(r *tester.Result) bool {
		return r.Fail || r.Error != nil
	})
}

func runRegoTests(ctx context.Context, modules map[string]*ast.Module, store storage.Store) ([]*tester.Result, error) {
	ch, err := tester.NewRunner().SetStore(store).SetModules(modules).RunTests(ctx, nil)
	if err != nil {
		return nil, err
	}
	var results []*tester.Result
	for r := range ch {
		results = append(results, r)
	}
	return results, nil
}

func hasRegoTestRule(module *ast.Module) bool {
	return slices.ContainsFunc(module.Rules, func(rule *ast.Rule) bool {
		return strings.HasPrefix(rule.Head.Ref().String(), tester.TestPrefix)
	})
}

func regoTestName(r *tester.Result) string {
	return r.Package + "." + r.Name
}
