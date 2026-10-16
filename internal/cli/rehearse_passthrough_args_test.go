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
)

// passthroughFixture is a root at <rehearsal>/work/prod with the copy
// <rehearsal>/work as its boundary, a dir outside the copy, and in the
// root: link (a dir symlink to outside), leaflink (a file symlink to
// outside/f) and old.tfplan (a regular file). withVars writes
// <rehearsal>/vars.tfvars. The replacer fills <root>, <outside> and
// <var-file> in a case's args and wants.
func passthroughFixture(t *testing.T, withVars bool) (rehearsalPassthroughDirs, *strings.Replacer) {
	t.Helper()
	tmp := t.TempDir()
	rehearsal := filepath.Join(tmp, "id")
	dirs := rehearsalPassthroughDirs{root: filepath.Join(rehearsal, "work", "prod"), boundary: filepath.Join(rehearsal, "work"), rehearsal: rehearsal}
	outside := filepath.Join(tmp, "outside")
	require.NoError(t, os.MkdirAll(dirs.root, 0o700))
	require.NoError(t, os.MkdirAll(outside, 0o700))
	mustWriteFile(t, filepath.Join(outside, "f"), "")
	mustWriteFile(t, filepath.Join(dirs.root, "old.tfplan"), "")
	require.NoError(t, os.Symlink(outside, filepath.Join(dirs.root, "link")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "f"), filepath.Join(dirs.root, "leaflink")))
	varFile := filepath.Join(rehearsal, rehearsalVarFile)
	if withVars {
		mustWriteFile(t, varFile, "x = 1\n")
	}
	return dirs, strings.NewReplacer("<root>", dirs.root, "<outside>", outside, "<var-file>", "-var-file="+varFile)
}

// Each case gives tofu's exact argv, or a refusal naming what it refuses.
func TestRehearsePassthroughArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, action string
		args         []string
		vars         bool
		want         []string
		refused      string
	}{
		{name: "-var x=y", action: "plan", args: []string{"-var", "x=y"}, refused: "-var is refused"},
		{name: "-var=x=y", action: "plan", args: []string{"-var=x=y"}, refused: "-var is refused"},
		{name: "--var=x=y", action: "plan", args: []string{"--var=x=y"}, refused: "-var is refused"},
		{name: "-var-file", action: "plan", args: []string{"-var-file=f"}, refused: "-var-file is refused"},
		{name: "-input=true", action: "plan", args: []string{"-input=true"}, refused: "-input is refused"},
		{name: "-state", action: "plan", args: []string{"-state=s"}, refused: "-state is refused"},
		{name: "-state-out", action: "apply", args: []string{"-state-out=s"}, refused: "-state-out is refused"},
		{name: "-backup", action: "apply", args: []string{"-backup=b"}, refused: "-backup is refused"},
		{name: "-chdir", action: "plan", args: []string{"-chdir=d"}, refused: "-chdir is refused"},
		{name: "-plugin-dir", action: "plan", args: []string{"-plugin-dir=d"}, refused: "-plugin-dir is refused"},
		{name: "-backend-config", action: "plan", args: []string{"-backend-config=b"}, refused: "-backend-config is refused"},
		{name: "-generate-config-out", action: "plan", args: []string{"-generate-config-out=g.tf"}, refused: "-generate-config-out is refused"},
		{name: "unknown -foo", action: "plan", args: []string{"-foo"}, refused: "-foo is refused"},
		{name: "lone dash", action: "plan", args: []string{"-"}, refused: `"-" is refused: it is not a flag`},
		{name: "-- inside the args", action: "plan", args: []string{"-out=p", "--", "-var=x=y"}, refused: "a -- inside the tofu args is refused"},
		{name: "plan -json-into", action: "plan", args: []string{"-json-into=x"}, refused: "-json-into is refused"},
		{name: "apply -json-into", action: "apply", args: []string{"-json-into=x"}, refused: "-json-into is refused"},
		{name: "import -json-into", action: "import", args: []string{"-json-into=x", "a", "b"}, refused: "-json-into is refused"},
		{name: "state mv -json-into", action: "state mv", args: []string{"-json-into=x", "a", "b"}, refused: "-json-into is refused"},
		{name: "import -config", action: "import", args: []string{"-config=d", "a", "b"}, refused: "-config is refused"},
		{name: "-out=../../x", action: "plan", args: []string{"-out=../../x"}, refused: "-out=../../x is refused"},
		{name: "-out=link/../p", action: "plan", args: []string{"-out=link/../p"}, refused: "-out=link/../p is refused: a .. element"},
		{name: "-out=/abs/p", action: "plan", args: []string{"-out=<outside>/p"}, refused: "-out=<outside>/p is refused: its dir"},
		{name: "-out=link/p", action: "plan", args: []string{"-out=link/p"}, refused: "-out=link/p is refused: its dir"},
		{name: "-out symlink leaf", action: "plan", args: []string{"-out=leaflink"}, refused: "-out=leaflink is refused: it is a symlink"},
		{name: "-out dir", action: "plan", args: []string{"-out=."}, refused: "-out=. is refused: it is not a regular file"},
		{name: "-out=main.tf", action: "plan", args: []string{"-out=main.tf"}, refused: "-out=main.tf is refused: tofu loads .tf files"},
		{name: "-out=MAIN.TF", action: "plan", args: []string{"-out=MAIN.TF"}, refused: "-out=MAIN.TF is refused: tofu loads .tf files"},
		{name: "-out=x.auto.tfvars", action: "plan", args: []string{"-out=x.auto.tfvars"}, refused: "-out=x.auto.tfvars is refused: tofu loads .tfvars files"},
		{name: "-out=x.tf.json", action: "plan", args: []string{"-out=x.tf.json"}, refused: "-out=x.tf.json is refused"},
		{name: "-out=x.tfvars.json", action: "plan", args: []string{"-out=x.tfvars.json"}, refused: "-out=x.tfvars.json is refused"},
		{name: "-out=infrafactory_override.tf", action: "plan", args: []string{"-out=infrafactory_override.tf"}, refused: "-out=infrafactory_override.tf is refused: it is infrafactory's own override"},
		{name: "-out=infrafactory_aws_override.tf", action: "plan", args: []string{"-out=infrafactory_aws_override.tf"}, refused: "-out=infrafactory_aws_override.tf is refused: it is infrafactory's own override"},
		{name: "-out=terraform.d", action: "plan", args: []string{"-out=terraform.d"}, refused: "-out=terraform.d is refused: a terraform.d entry"},
		{name: "-out with no value", action: "plan", args: []string{"-out"}, refused: "-out is refused: it needs a value"},
		{name: "apply plan file", action: "apply", args: []string{"old.tfplan"}, vars: true, refused: `the plan file "old.tfplan" is refused`},
		{name: "import one positional", action: "import", args: []string{"a"}, refused: "tofu import takes 2 positional arguments, not 1"},
		{name: "plan positional", action: "plan", args: []string{"-lock", "false"}, refused: "tofu plan takes 0 positional arguments, not 1"},
		{name: "unknown action", action: "refresh", refused: "tofu refresh is refused"},
		{name: "state rm", action: "state rm", args: []string{"a"}, refused: "tofu state rm is refused"},

		{name: "-out=p", action: "plan", args: []string{"-out=p"}, vars: true, want: []string{"plan", "-input=false", "<var-file>", "-out=p"}},
		{name: "-out p", action: "plan", args: []string{"-out", "p"}, vars: true, want: []string{"plan", "-input=false", "<var-file>", "-out", "p"}},
		{name: "-out inside, absolute", action: "plan", args: []string{"-out=<root>/p"}, want: []string{"plan", "-input=false", "-out=<root>/p"}},
		{name: "-out over a plan file", action: "plan", args: []string{"--out=old.tfplan", "--lock=false"}, want: []string{"plan", "-input=false", "--out=old.tfplan", "--lock=false"}},
		{name: "plan without vars", action: "plan", args: []string{"-lock-timeout", "5s"}, want: []string{"plan", "-input=false", "-lock-timeout", "5s"}},
		{name: "apply", action: "apply", args: []string{"-auto-approve"}, vars: true, want: []string{"apply", "-input=false", "<var-file>", "-auto-approve"}},
		{name: "apply without vars", action: "apply", args: []string{"-auto-approve"}, want: []string{"apply", "-input=false", "-auto-approve"}},
		{name: "destroy", action: "destroy", vars: true, want: []string{"destroy", "-input=false", "<var-file>"}},
		{name: "destroy without vars", action: "destroy", want: []string{"destroy", "-input=false"}},
		{name: "import", action: "import", args: []string{"aws_vpc.main", "vpc-1"}, vars: true, want: []string{"import", "-input=false", "<var-file>", "aws_vpc.main", "vpc-1"}},
		{name: "import without vars", action: "import", args: []string{"aws_vpc.main", "vpc-1"}, want: []string{"import", "-input=false", "aws_vpc.main", "vpc-1"}},
		{name: "console", action: "console", vars: true, want: []string{"console", "-input=false", "<var-file>"}},
		{name: "console without vars", action: "console", want: []string{"console", "-input=false"}},
		{name: "state mv", action: "state mv", args: []string{"-dry-run", "aws_vpc.a", "aws_vpc.b"}, vars: true, want: []string{"state", "mv", "-dry-run", "aws_vpc.a", "aws_vpc.b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dirs, fill := passthroughFixture(t, tc.vars)
			args := make([]string, len(tc.args))
			for i, arg := range tc.args {
				args[i] = fill.Replace(arg)
			}
			got, err := rehearsalPassthroughArgs(tc.action, args, dirs)
			if tc.refused != "" {
				assert.ErrorContains(t, err, fill.Replace(tc.refused))
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			want := make([]string, len(tc.want))
			for i, arg := range tc.want {
				want[i] = fill.Replace(arg)
			}
			assert.Equal(t, want, got)
		})
	}
}

// Each action's allowlist in full: every bool is accepted as -x and every
// value as -x v, the value taken as the flag's and not as a positional.
func TestRehearsePassthroughArgsAllowlist(t *testing.T) {
	t.Parallel()
	allow := map[string]struct{ bools, values []string }{
		"plan": {
			bools:  []string{"destroy", "refresh-only", "refresh", "detailed-exitcode", "lock", "no-color", "concise", "compact-warnings", "consolidate-warnings", "consolidate-errors"},
			values: []string{"out", "target", "exclude", "replace", "lock-timeout", "parallelism"},
		},
		"apply": {
			bools:  []string{"auto-approve", "refresh-only", "refresh", "lock", "no-color", "concise", "compact-warnings", "consolidate-warnings", "consolidate-errors"},
			values: []string{"target", "exclude", "replace", "lock-timeout", "parallelism"},
		},
		"destroy": {
			bools:  []string{"auto-approve", "refresh", "lock", "no-color", "concise", "compact-warnings", "consolidate-warnings", "consolidate-errors"},
			values: []string{"target", "exclude", "lock-timeout", "parallelism"},
		},
		"import":   {bools: []string{"lock", "no-color", "compact-warnings", "consolidate-warnings", "consolidate-errors"}, values: []string{"lock-timeout"}},
		"state mv": {bools: []string{"dry-run", "lock"}, values: []string{"lock-timeout"}},
		"console":  {bools: []string{"lock", "compact-warnings", "consolidate-warnings", "consolidate-errors"}, values: []string{"lock-timeout"}},
	}
	dirs, _ := passthroughFixture(t, false)
	assert.ElementsMatch(t, slices.Collect(maps.Keys(allow)), slices.Collect(maps.Keys(rehearsalPassthroughActions)))
	for action, want := range allow {
		a := rehearsalPassthroughActions[action]
		assert.ElementsMatch(t, want.bools, a.bools, action)
		assert.ElementsMatch(t, want.values, a.values, action)
		positionals := slices.Repeat([]string{"a"}, a.positionals)
		for _, args := range slices.Concat(flagArgs(want.bools), flagArgs(want.values, "p")) {
			args = append(args, positionals...)
			got, err := rehearsalPassthroughArgs(action, args, dirs)
			if assert.NoError(t, err, "%s %q", action, args) {
				assert.Equal(t, args, got[len(got)-len(args):], "%s %q", action, args)
			}
		}
	}
}

// flagArgs is one arg list per name: -name, then value when there is one.
func flagArgs(names []string, value ...string) [][]string {
	var all [][]string
	for _, name := range names {
		all = append(all, append([]string{"-" + name}, value...))
	}
	return all
}
