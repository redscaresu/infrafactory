package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"sort"

	"github.com/redscaresu/infrafactory/internal/harness"
)

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, cmd harness.Command) (harness.CommandResult, error) {
	execCmd := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	execCmd.Dir = cmd.Dir
	execCmd.Env = withEnvOverrides(os.Environ(), cmd.Env)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	execCmd.Stdout = &stdout
	execCmd.Stderr = &stderr

	err := execCmd.Run()
	return harness.CommandResult{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}, err
}

func withEnvOverrides(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}

	overridden := make(map[string]struct{}, len(overrides))
	pairs := make([]string, 0, len(overrides))
	for key, value := range overrides {
		overridden[key] = struct{}{}
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)

	out := make([]string, 0, len(base)+len(pairs))
	for _, entry := range base {
		key := entry
		if idx := bytes.IndexByte([]byte(entry), '='); idx >= 0 {
			key = entry[:idx]
		}
		if _, ok := overridden[key]; ok {
			continue
		}
		out = append(out, entry)
	}
	out = append(out, pairs...)

	return out
}
