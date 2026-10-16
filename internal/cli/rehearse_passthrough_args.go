package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// rehearsalPassthroughAction is what infrafactory lets the user pass to
// one tofu action: its allowed flags, bools (-x or -x=v) and values (-x=v
// or -x v), how many positional arguments it takes, and whether
// -input=false and the rehearsal's var file go before the user's args.
type rehearsalPassthroughAction struct {
	bools, values []string
	positionals   int
	inject        bool
}

// rehearsalPassthroughActions is every passthrough action and its whole
// allowlist. Any other flag is refused by name: -var and -var-file (the
// rehearsal passes prepare's var file and no other), -input (-input=false
// is passed), and every flag that names a state, backup, config, chdir,
// plugin or output path (-state, -state-out, -backup, -config, -chdir,
// -plugin-dir, -backend-config, -generate-config-out, -json-into), whose
// path nothing checks. Only plan's -out takes a path, and
// rehearsalPassthroughOut checks it. Apply takes no saved plan file: the
// re-gate checks the source, not the configuration inside a plan
// (ADR-0042), so apply runs from source.
var rehearsalPassthroughActions = map[string]rehearsalPassthroughAction{
	"plan": {
		bools:  []string{"destroy", "refresh-only", "refresh", "detailed-exitcode", "lock", "no-color", "concise", "compact-warnings", "consolidate-warnings", "consolidate-errors"},
		values: []string{"out", "target", "exclude", "replace", "lock-timeout", "parallelism"},
		inject: true,
	},
	"apply": {
		bools:  []string{"auto-approve", "refresh-only", "refresh", "lock", "no-color", "concise", "compact-warnings", "consolidate-warnings", "consolidate-errors"},
		values: []string{"target", "exclude", "replace", "lock-timeout", "parallelism"},
		inject: true,
	},
	"destroy": {
		bools:  []string{"auto-approve", "refresh", "lock", "no-color", "concise", "compact-warnings", "consolidate-warnings", "consolidate-errors"},
		values: []string{"target", "exclude", "lock-timeout", "parallelism"},
		inject: true,
	},
	"import": {
		bools:       []string{"lock", "no-color", "compact-warnings", "consolidate-warnings", "consolidate-errors"},
		values:      []string{"lock-timeout"},
		positionals: 2,
		inject:      true,
	},
	"state mv": {
		bools:       []string{"dry-run", "lock"},
		values:      []string{"lock-timeout"},
		positionals: 2,
	},
	"console": {
		bools:  []string{"lock", "compact-warnings", "consolidate-warnings", "consolidate-errors"},
		values: []string{"lock-timeout"},
		inject: true,
	},
}

// rehearsalPassthroughDirs is where a passthrough action runs: root is
// tofu's cwd, boundary the copy a path must stay inside, and rehearsal
// the <id> dir that holds the var file.
type rehearsalPassthroughDirs struct {
	root, boundary, rehearsal string
}

// rehearsalPassthroughOutSuffixes are -out names tofu would load as
// configuration or variables on the next run.
var rehearsalPassthroughOutSuffixes = []string{".tf", ".tf.json", ".tfvars", ".tfvars.json"}

// rehearsalPassthroughArgs is tofu's argv for action with the user's args
// (those after `--`), or the refusal naming what it refuses. -input=false
// and, when <rehearsal>/vars.tfvars exists, -var-file=<it> go before the
// user's args on every action that injects. The user's args are passed as
// written.
func rehearsalPassthroughArgs(action string, args []string, dirs rehearsalPassthroughDirs) ([]string, error) {
	a, ok := rehearsalPassthroughActions[action]
	if !ok {
		return nil, fmt.Errorf("tofu %s is refused: the rehearsal passes only %s", action, strings.Join(slices.Sorted(maps.Keys(rehearsalPassthroughActions)), ", "))
	}
	if slices.Contains(args, "--") {
		return nil, fmt.Errorf("tofu %s: a -- inside the tofu args is refused: only the first -- separates them from infrafactory's", action)
	}
	positionals, err := a.check(action, args, dirs)
	if err != nil {
		return nil, err
	}
	switch {
	case action == "apply" && len(positionals) > 0:
		return nil, fmt.Errorf("tofu apply: the plan file %q is refused: the re-gate checks the source, not the configuration inside a saved plan, so apply runs from source", positionals[0])
	case len(positionals) != a.positionals:
		return nil, fmt.Errorf("tofu %s takes %d positional arguments, not %d: %q", action, a.positionals, len(positionals), positionals)
	}
	argv := strings.Fields(action)
	if a.inject {
		injected, err := rehearsalPassthroughInjected(dirs.rehearsal)
		if err != nil {
			return nil, err
		}
		argv = append(argv, injected...)
	}
	return append(argv, args...), nil
}

// check refuses any flag a does not allow, a value flag with no value and
// a refused -out, and returns the positional arguments.
func (a rehearsalPassthroughAction) check(action string, args []string, dirs rehearsalPassthroughDirs) ([]string, error) {
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			positionals = append(positionals, arg)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "=")
		switch {
		case name == "" || strings.HasPrefix(name, "-"):
			return nil, fmt.Errorf("tofu %s: %q is refused: it is not a flag", action, arg)
		case slices.Contains(a.bools, name):
			continue
		case !slices.Contains(a.values, name):
			return nil, fmt.Errorf("tofu %s: -%s is refused: %s allows only -%s", action, name, action, strings.Join(slices.Concat(a.bools, a.values), ", -"))
		case !hasValue && i+1 == len(args):
			return nil, fmt.Errorf("tofu %s: -%s is refused: it needs a value", action, name)
		case !hasValue:
			i++
			value = args[i]
		}
		if name == "out" {
			if problem := rehearsalPassthroughOut(value, dirs); problem != "" {
				return nil, fmt.Errorf("tofu %s: -out=%s is refused: %s", action, value, problem)
			}
		}
	}
	return positionals, nil
}

// rehearsalPassthroughOut is why the plan file out is refused, or "".
// A relative out is joined onto the root, tofu's cwd. No .. element is
// allowed, since tofu resolves link/.. through the link where Join cleans
// it away. Its dir must resolve to itself inside the boundary; the file
// must be absent or a regular file, never a symlink; and its name must be
// none tofu would load next run, nor one of infrafactory's own overrides.
func rehearsalPassthroughOut(out string, dirs rehearsalPassthroughDirs) string {
	if slices.Contains(strings.Split(filepath.ToSlash(out), "/"), "..") {
		return "a .. element is refused"
	}
	path := out
	if !filepath.IsAbs(path) {
		path = filepath.Join(dirs.root, path)
	}
	if problem := rehearsalResolvesInside(filepath.Dir(path), dirs.boundary); problem != "" {
		return "its dir " + problem
	}
	switch info, err := os.Lstat(path); {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return fmt.Sprintf("cannot be checked: %v", err)
	case info.Mode()&fs.ModeSymlink != 0:
		return "it is a symlink"
	case !info.Mode().IsRegular():
		return "it is not a regular file"
	}
	lower := strings.ToLower(filepath.Base(path))
	if _, ours := (rehearsalRecord{}).ours(rehearsalRoot{})[lower]; ours {
		return "it is infrafactory's own override"
	}
	if problem := rehearsalEntryProblem(lower); problem != "" {
		return problem
	}
	for _, suffix := range rehearsalPassthroughOutSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return fmt.Sprintf("tofu loads %s files from the root", suffix)
		}
	}
	return ""
}

// rehearsalPassthroughInjected is -input=false, then -var-file=<it> when
// <rehearsal>/vars.tfvars exists.
func rehearsalPassthroughInjected(rehearsal string) ([]string, error) {
	injected := []string{"-input=false"}
	varFile := filepath.Join(rehearsal, rehearsalVarFile)
	switch _, err := os.Lstat(varFile); {
	case err == nil:
		injected = append(injected, "-var-file="+varFile)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("cannot look for %s: %w", varFile, err)
	}
	return injected, nil
}
