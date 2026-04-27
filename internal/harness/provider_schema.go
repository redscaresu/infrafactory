package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// providersTFForCloud returns the providers.tf content for the given
// cloud. Per concepts.md "Required surface" item 9 (S43-T9): the
// extractor was scaleway-only before; now it dispatches per cloud so
// `cloud:aws` scenarios get the AWS provider schema.
//
// Provider version pin per fakeaws/concepts.md "Resolved decisions"
// item 14: hashicorp/aws ~> 5.70. Bumps require a single PR
// updating this string + the example required_providers blocks +
// the prompt templates together.
func providersTFForCloud(cloud string) string {
	switch cloud {
	case "aws":
		return `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
}
`
	case "gcp":
		return `terraform {
  required_providers {
    google = {
      source = "hashicorp/google"
    }
  }
}
`
	default: // scaleway and unknown clouds keep the historical default
		return `terraform {
  required_providers {
    scaleway = {
      source = "scaleway/scaleway"
    }
  }
}
`
	}
}

// ExtractProviderSchema runs tofu init and tofu providers schema -json in a
// temporary directory to obtain the full provider schema JSON. The caller
// receives raw JSON bytes suitable for filtering down to specific resource
// types before injecting into prompts.
//
// Backwards-compatible signature: omitting cloud (or passing "") uses
// the historical scaleway default. New callers should pass the active
// scenario's cloud — see ExtractProviderSchemaForCloud below for the
// preferred entry point.
func ExtractProviderSchema(ctx context.Context, runner CommandRunner, env map[string]string) ([]byte, error) {
	return ExtractProviderSchemaForCloud(ctx, runner, env, "scaleway")
}

// ExtractProviderSchemaForCloud is the cloud-aware entry point for
// per-scenario provider-schema extraction. cloud is one of
// "scaleway", "gcp", "aws"; unknown values fall back to scaleway.
//
// Per concepts.md "Required surface" item 9: the call site
// (CommandRuntime.EnsureProviderSchema in internal/cli/runtime.go) was
// restructured so extraction runs lazily after LoadScenario. Pre-S43-T9
// behaviour (extract once at runtime construction) is no longer
// reachable — the runtime now caches by cloud.
func ExtractProviderSchemaForCloud(ctx context.Context, runner CommandRunner, env map[string]string, cloud string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "infrafactory-schema-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir for schema extraction: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "providers.tf"), []byte(providersTFForCloud(cloud)), 0o644); err != nil {
		return nil, fmt.Errorf("write providers.tf for schema extraction: %w", err)
	}

	initResult, err := runner.Run(ctx, Command{
		Name: "tofu",
		Args: []string{"init"},
		Dir:  tmpDir,
		Env:  env,
	})
	if err != nil {
		return nil, fmt.Errorf("tofu init for schema extraction: %w (stderr: %s)", err, string(initResult.Stderr))
	}

	schemaResult, err := runner.Run(ctx, Command{
		Name: "tofu",
		Args: []string{"providers", "schema", "-json"},
		Dir:  tmpDir,
		Env:  env,
	})
	if err != nil {
		return nil, fmt.Errorf("tofu providers schema -json: %w (stderr: %s)", err, string(schemaResult.Stderr))
	}

	return schemaResult.Stdout, nil
}
