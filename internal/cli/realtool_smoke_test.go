package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestValidateCommandRealToolSmoke(t *testing.T) {
	if os.Getenv("INFRAFACTORY_ENABLE_REALTOOL_SMOKE") != "1" {
		t.Skip("set INFRAFACTORY_ENABLE_REALTOOL_SMOKE=1 to enable real-tool smoke tests")
	}
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary is required for real-tool smoke: %v", err)
	}

	workspace := t.TempDir()
	scenarioPath := filepath.Join(workspace, "scenarios", "training", "smoke.yaml")
	outputRoot := filepath.Join(workspace, "output")
	outputDir := filepath.Join(outputRoot, "smoke-scenario")
	configPath := filepath.Join(workspace, "infrafactory.yaml")

	mustWriteFile(t, scenarioPath, `scenario: smoke-scenario
version: "1.0"
cloud: scaleway
description: smoke
resources:
  compute:
    purpose: smoke
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`)
	mustWriteFile(t, filepath.Join(outputDir, "main.tf"), `terraform {}
`)
	mustWriteFile(t, configPath, `version: "1.0"
agent:
  type: claude-code
mockway:
  url: http://localhost:8080
paths:
  output: `+outputRoot+`
validation:
  layers:
    static:
      enabled: true
      policy_paths: []
    mock_deploy:
      enabled: false
    sandbox_deploy:
      enabled: false
    destruction:
      enabled: false
`)

	root := NewRootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"validate", scenarioPath, "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("validate smoke failed: %v\noutput:\n%s", err, stdout.String())
	}
}

func TestTestCommandRealToolMockwaySmoke(t *testing.T) {
	if os.Getenv("INFRAFACTORY_ENABLE_REALTOOL_MOCKWAY") != "1" {
		t.Skip("set INFRAFACTORY_ENABLE_REALTOOL_MOCKWAY=1 to enable mockway smoke")
	}
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary is required for mockway smoke: %v", err)
	}
	mockwayURL := os.Getenv("INFRAFACTORY_MOCKWAY_URL")
	if mockwayURL == "" {
		t.Fatal("set INFRAFACTORY_MOCKWAY_URL for mockway smoke")
	}

	workspace := t.TempDir()
	scenarioPath := filepath.Join(workspace, "scenarios", "training", "smoke-test.yaml")
	outputRoot := filepath.Join(workspace, "output")
	outputDir := filepath.Join(outputRoot, "smoke-test")
	configPath := filepath.Join(workspace, "infrafactory.yaml")

	mustWriteFile(t, scenarioPath, `scenario: smoke-test
version: "1.0"
cloud: scaleway
description: smoke
resources:
  compute:
    purpose: smoke
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`)
	mustWriteFile(t, filepath.Join(outputDir, "main.tf"), `terraform {}
`)
	mustWriteFile(t, configPath, `version: "1.0"
agent:
  type: claude-code
mockway:
  url: `+mockwayURL+`
paths:
  output: `+outputRoot+`
validation:
  layers:
    static:
      enabled: false
      policy_paths: []
    mock_deploy:
      enabled: true
    sandbox_deploy:
      enabled: false
    destruction:
      enabled: true
`)

	root := NewRootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"test", scenarioPath, "--config", configPath})

	if err := root.Execute(); err != nil {
		t.Fatalf("test smoke failed: %v\noutput:\n%s", err, stdout.String())
	}
}
