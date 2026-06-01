package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/redscaresu/infrafactory/internal/harness"
)

type execCommandRunner struct{}

// gcpAuthEnvPrefixes lists env vars that trigger terraform-provider-google's
// Application Default Credentials probing. When the parent process (the
// developer's shell, CI runner, or a previous `gcloud auth login`) has
// any of these set, the v5 SDK skips the access_token short-circuit and
// instead probes the metadata server / token-exchange endpoint, which 401s
// against fakegcp's bearer-token middleware and surfaces as
// `ACCESS_TOKEN_TYPE_UNSUPPORTED`. Stripping them at the harness boundary
// guarantees the LLM's providers.tf credentials field is what's used.
//
// Surfaced 2026-06-02 by the GCP investigation agent — even with all 18
// _custom_endpoint flags set correctly, gcp-cloud-run's
// google_project_service preflight escaped to real cloudresourcemanager.
// Same pattern as the CLAUDECODE strip in claude_adapter.go.
var gcpAuthEnvPrefixes = []string{
	"GOOGLE_APPLICATION_CREDENTIALS",
	"GOOGLE_CREDENTIALS",
	"GOOGLE_CLOUD_KEYFILE_JSON",
	"GOOGLE_OAUTH_ACCESS_TOKEN",
	"CLOUDSDK_",
	"GCLOUD_",
}

func stripGCPAuthEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		idx := strings.IndexByte(entry, '=')
		if idx < 0 {
			out = append(out, entry)
			continue
		}
		key := entry[:idx]
		drop := false
		for _, prefix := range gcpAuthEnvPrefixes {
			if key == prefix || strings.HasPrefix(key, prefix) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, entry)
		}
	}
	return out
}

func (execCommandRunner) Run(ctx context.Context, cmd harness.Command) (harness.CommandResult, error) {
	execCmd := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	execCmd.Dir = cmd.Dir
	execCmd.Env = withEnvOverrides(stripGCPAuthEnv(os.Environ()), cmd.Env)

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
