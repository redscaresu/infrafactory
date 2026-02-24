package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const scalewayProvidersTF = `terraform {
  required_providers {
    scaleway = {
      source = "scaleway/scaleway"
    }
  }
}
`

// ExtractProviderSchema runs tofu init and tofu providers schema -json in a
// temporary directory to obtain the full provider schema JSON. The caller
// receives raw JSON bytes suitable for filtering down to specific resource
// types before injecting into prompts.
func ExtractProviderSchema(ctx context.Context, runner CommandRunner, env map[string]string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "infrafactory-schema-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir for schema extraction: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "providers.tf"), []byte(scalewayProvidersTF), 0o644); err != nil {
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
