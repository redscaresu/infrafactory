package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
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

func TestRunCommandRealToolIncrementalMockwayE2E(t *testing.T) {
	if os.Getenv("INFRAFACTORY_ENABLE_REALTOOL_INCREMENTAL") != "1" {
		t.Skip("set INFRAFACTORY_ENABLE_REALTOOL_INCREMENTAL=1 to enable incremental mockway e2e")
	}
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary is required for incremental mockway e2e: %v", err)
	}
	mockwayURL, cleanup := startMockwayFromSource(t)
	defer cleanup()

	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	scenarioPath := filepath.Join(workspace, "scenarios", "training", "incremental-project-paris.yaml")

	writeIncrementalScenarioStage(t, scenarioPath, incrementalScenarioStage1YAML)
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
      enabled: true
      policy_paths: []
    mock_deploy:
      enabled: true
    sandbox_deploy:
      enabled: false
    destruction:
      enabled: true
`)

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(_ context.Context, req generator.Request) (*generator.GeneratedCode, error) {
				stage := incrementalStageFromScenarioYAML(string(req.ScenarioYAML))
				return &generator.GeneratedCode{Files: incrementalStageFiles(stage)}, nil
			}),
			MockState: newMockwayStateClient(mockwayURL),
		},
	}

	run := func(args ...string) string {
		cmd := newRunCommandForTest(opts)
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("run command failed: %v\noutput:\n%s", err, stdout.String())
		}
		return stdout.String()
	}

	stage1Output := run(scenarioPath, "--config", configPath, "--no-destroy")
	assertContains(t, stage1Output, "run/mode: pass (clean")
	stage1State := fetchMockState(t, mockwayURL)
	stage1Servers := resourceIDs(stage1State, "instance", "servers")
	stage1DBs := resourceIDs(stage1State, "rdb", "instances")
	stage1Redis := resourceIDs(stage1State, "redis", "clusters")
	if len(stage1Servers) != 2 || len(stage1DBs) != 0 || len(stage1Redis) != 0 {
		t.Fatalf("unexpected stage 1 state: servers=%v dbs=%v redis=%v", stage1Servers, stage1DBs, stage1Redis)
	}

	writeIncrementalScenarioStage(t, scenarioPath, incrementalScenarioStage2YAML)
	stage2Output := run(scenarioPath, "--config", configPath, "--no-destroy")
	assertContains(t, stage2Output, "run/mode: pass (incremental")
	stage2State := fetchMockState(t, mockwayURL)
	stage2Servers := resourceIDs(stage2State, "instance", "servers")
	stage2DBs := resourceIDs(stage2State, "rdb", "instances")
	if !sameStrings(stage1Servers, stage2Servers) {
		t.Fatalf("expected server ids to persist across incremental stage 2: before=%v after=%v", stage1Servers, stage2Servers)
	}
	if len(stage2DBs) != 1 {
		t.Fatalf("expected one database after stage 2, got %v", stage2DBs)
	}

	writeIncrementalScenarioStage(t, scenarioPath, incrementalScenarioStage3YAML)
	stage3Output := run(scenarioPath, "--config", configPath, "--no-destroy")
	assertContains(t, stage3Output, "run/mode: pass (incremental")
	stage3State := fetchMockState(t, mockwayURL)
	stage3Servers := resourceIDs(stage3State, "instance", "servers")
	stage3DBs := resourceIDs(stage3State, "rdb", "instances")
	stage3Redis := resourceIDs(stage3State, "redis", "clusters")
	if !sameStrings(stage2Servers, stage3Servers) {
		t.Fatalf("expected server ids to persist across incremental stage 3: before=%v after=%v", stage2Servers, stage3Servers)
	}
	if !sameStrings(stage2DBs, stage3DBs) {
		t.Fatalf("expected database ids to persist across incremental stage 3: before=%v after=%v", stage2DBs, stage3DBs)
	}
	if len(stage3Redis) != 1 {
		t.Fatalf("expected one redis cluster after stage 3, got %v", stage3Redis)
	}

	cleanForcedOutput := run(scenarioPath, "--config", configPath, "--clean")
	assertContains(t, cleanForcedOutput, "run/mode: pass (clean (forced by --clean))")
	for _, ids := range [][]string{
		resourceIDs(fetchMockState(t, mockwayURL), "instance", "servers"),
		resourceIDs(fetchMockState(t, mockwayURL), "rdb", "instances"),
		resourceIDs(fetchMockState(t, mockwayURL), "redis", "clusters"),
	} {
		if len(ids) != 0 {
			t.Fatalf("expected forced clean run to finish with destroyed state, got %v", ids)
		}
	}

	reseedOutput := run(scenarioPath, "--config", configPath, "--no-destroy")
	assertContains(t, reseedOutput, "run/mode: pass (clean")
	reseedState := fetchMockState(t, mockwayURL)
	if len(resourceIDs(reseedState, "redis", "clusters")) != 1 {
		t.Fatalf("expected reseed run to recreate redis after clean destroy")
	}

	finalDestroyOutput := run(scenarioPath, "--config", configPath)
	assertContains(t, finalDestroyOutput, "run/mode: pass (incremental")
	finalState := fetchMockState(t, mockwayURL)
	for _, tc := range []struct {
		root       string
		collection string
	}{
		{root: "instance", collection: "servers"},
		{root: "rdb", collection: "instances"},
		{root: "redis", collection: "clusters"},
		{root: "lb", collection: "lbs"},
		{root: "vpc", collection: "vpcs"},
	} {
		if got := resourceIDs(finalState, tc.root, tc.collection); len(got) != 0 {
			t.Fatalf("expected empty final state for %s/%s, got %v", tc.root, tc.collection, got)
		}
	}

	postDestroyOutput := run(scenarioPath, "--config", configPath, "--no-destroy")
	assertContains(t, postDestroyOutput, "run/mode: pass (clean")
}

func TestTestCommandRealToolLayer3Smoke(t *testing.T) {
	if os.Getenv("INFRAFACTORY_ENABLE_REALTOOL_LAYER3") != "1" {
		t.Skip("set INFRAFACTORY_ENABLE_REALTOOL_LAYER3=1 to enable Layer 3 smoke")
	}
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary is required for Layer 3 smoke: %v", err)
	}
	if strings.TrimSpace(os.Getenv("SCW_ACCESS_KEY")) == "" || strings.TrimSpace(os.Getenv("SCW_SECRET_KEY")) == "" {
		t.Fatal("set SCW_ACCESS_KEY and SCW_SECRET_KEY for Layer 3 smoke")
	}
	mockwayURL, cleanup := startMockwayFromSource(t)
	defer cleanup()

	workspace := t.TempDir()
	scenarioPath := filepath.Join(workspace, "scenarios", "training", "layer3-smoke.yaml")
	outputRoot := filepath.Join(workspace, "output")
	configPath := filepath.Join(workspace, "infrafactory.yaml")

	mustWriteFile(t, scenarioPath, `scenario: layer3-smoke
version: "1.0"
cloud: scaleway
description: Layer 3 real smoke
resources:
  compute:
    purpose: web
    size: small
  networking:
    vpc: true
    private_network: true
    load_balancer:
      exposure: public
      backends:
        - port: 80
          protocol: http
acceptance_criteria:
  - type: destruction
    expect: no_orphans
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
      enabled: true
      policy_paths: []
    mock_deploy:
      enabled: true
    sandbox_deploy:
      enabled: true
    destruction:
      enabled: true
`)

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(_ context.Context, req generator.Request) (*generator.GeneratedCode, error) {
				_ = req
				return &generator.GeneratedCode{Files: incrementalStageFiles(1)}, nil
			}),
			MockState: newMockwayStateClient(mockwayURL),
		},
	}

	cmd := newTestCommandForTest(opts)
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{scenarioPath, "--config", configPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("layer3 smoke failed: %v\noutput:\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "- sandbox_deploy/apply: pass") {
		t.Fatalf("expected sandbox apply stage, got:\n%s", stdout.String())
	}
}

func TestRunCommandRealToolLayer3IncrementalE2E(t *testing.T) {
	if os.Getenv("INFRAFACTORY_ENABLE_REALTOOL_LAYER3_INCREMENTAL") != "1" {
		t.Skip("set INFRAFACTORY_ENABLE_REALTOOL_LAYER3_INCREMENTAL=1 to enable Layer 3 incremental e2e")
	}
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary is required for Layer 3 incremental e2e: %v", err)
	}
	if strings.TrimSpace(os.Getenv("SCW_ACCESS_KEY")) == "" || strings.TrimSpace(os.Getenv("SCW_SECRET_KEY")) == "" {
		t.Fatal("set SCW_ACCESS_KEY and SCW_SECRET_KEY for Layer 3 incremental e2e")
	}
	mockwayURL, cleanup := startMockwayFromSource(t)
	defer cleanup()

	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	scenarioPath := filepath.Join(workspace, "scenarios", "training", "incremental-project-paris.yaml")

	writeIncrementalScenarioStage(t, scenarioPath, incrementalScenarioStage1YAML)
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
      enabled: true
      policy_paths: []
    mock_deploy:
      enabled: true
    sandbox_deploy:
      enabled: true
    destruction:
      enabled: true
`)

	opts := runtimeOptions{
		configLoader:   config.Load,
		scenarioLoader: defaultScenarioLoader,
		deps: RuntimeDependencies{
			Generator: generator.SeedGeneratorFunc(func(_ context.Context, req generator.Request) (*generator.GeneratedCode, error) {
				stage := incrementalStageFromScenarioYAML(string(req.ScenarioYAML))
				return &generator.GeneratedCode{Files: incrementalStageFiles(stage)}, nil
			}),
			MockState: newMockwayStateClient(mockwayURL),
		},
	}

	run := func(args ...string) string {
		cmd := newRunCommandForTest(opts)
		stdout := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("run command failed: %v\noutput:\n%s", err, stdout.String())
		}
		return stdout.String()
	}

	stage1Output := run(scenarioPath, "--config", configPath, "--no-destroy")
	assertContains(t, stage1Output, "run/mode: pass (clean")

	writeIncrementalScenarioStage(t, scenarioPath, incrementalScenarioStage2YAML)
	stage2Output := run(scenarioPath, "--config", configPath, "--no-destroy")
	assertContains(t, stage2Output, "run/mode: pass (incremental")

	writeIncrementalScenarioStage(t, scenarioPath, incrementalScenarioStage3YAML)
	stage3Output := run(scenarioPath, "--config", configPath)
	assertContains(t, stage3Output, "run/mode: pass (incremental")
}

const incrementalScenarioStage1YAML = `scenario: incremental-project-paris
version: "1.0"
cloud: scaleway
description: Initial web and load balancer stage.
resources:
  compute:
    purpose: web-server
    size: small
    count: 2
  networking:
    vpc: true
    private_network: true
    load_balancer:
      exposure: public
      backends:
        - port: 80
          protocol: http
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`

const incrementalScenarioStage2YAML = `scenario: incremental-project-paris
version: "1.0"
cloud: scaleway
description: Add PostgreSQL to the existing web stack.
resources:
  compute:
    purpose: web-server
    size: small
    count: 2
  networking:
    vpc: true
    private_network: true
    load_balancer:
      exposure: public
      backends:
        - port: 80
          protocol: http
  database:
    engine: postgresql
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`

const incrementalScenarioStage3YAML = `scenario: incremental-project-paris
version: "1.0"
cloud: scaleway
description: Add Redis to the existing web and database stack.
resources:
  compute:
    purpose: web-server
    size: small
    count: 2
  networking:
    vpc: true
    private_network: true
    load_balancer:
      exposure: public
      backends:
        - port: 80
          protocol: http
  database:
    engine: postgresql
    size: small
  redis:
    purpose: cache
    size: small
acceptance_criteria:
  - type: destruction
    expect: no_orphans
`

func writeIncrementalScenarioStage(t *testing.T, path, content string) {
	t.Helper()
	mustWriteFile(t, path, content)
}

func incrementalStageFromScenarioYAML(yaml string) int {
	switch {
	case strings.Contains(yaml, "\n  redis:"):
		return 3
	case strings.Contains(yaml, "\n  database:"):
		return 2
	default:
		return 1
	}
}

func incrementalStageFiles(stage int) map[string][]byte {
	files := map[string][]byte{
		"providers.tf": []byte(`terraform {
  required_providers {
    scaleway = {
      source = "scaleway/scaleway"
    }
  }
}

provider "scaleway" {
  region = "fr-par"
  zone   = "fr-par-1"
}
`),
		"variables.tf": []byte(incrementalVariablesTF(stage)),
		"network.tf": []byte(`resource "scaleway_vpc" "main" {
  name   = "incremental-project-paris-vpc"
  region = var.region
}

resource "scaleway_vpc_private_network" "main" {
  name   = "incremental-project-paris-pn"
  vpc_id = scaleway_vpc.main.id
  region = var.region

  ipv4_subnet {
    subnet = "10.0.0.0/24"
  }
}
`),
		"compute.tf": []byte(`resource "scaleway_instance_ip" "web_0" {
  zone = var.zone
}

resource "scaleway_instance_ip" "web_1" {
  zone = var.zone
}

resource "scaleway_instance_server" "web_0" {
  name  = "web-0"
  type  = "DEV1-S"
  image = "ubuntu_jammy"
  ip_id = scaleway_instance_ip.web_0.id
  zone  = var.zone
}

resource "scaleway_instance_server" "web_1" {
  name  = "web-1"
  type  = "DEV1-S"
  image = "ubuntu_jammy"
  ip_id = scaleway_instance_ip.web_1.id
  zone  = var.zone
}

resource "scaleway_instance_private_nic" "web_0" {
  server_id          = scaleway_instance_server.web_0.id
  private_network_id = scaleway_vpc_private_network.main.id
}

resource "scaleway_instance_private_nic" "web_1" {
  server_id          = scaleway_instance_server.web_1.id
  private_network_id = scaleway_vpc_private_network.main.id
}
`),
		"loadbalancer.tf": []byte(`resource "scaleway_lb_ip" "main" {
  zone = var.zone
}

resource "scaleway_lb" "main" {
  name   = "incremental-project-paris-lb"
  ip_ids = [scaleway_lb_ip.main.id]
  type   = "LB-S"
  zone   = var.zone

  private_network {
    private_network_id = scaleway_vpc_private_network.main.id
  }
}

resource "scaleway_lb_backend" "main" {
  name             = "http-backend"
  lb_id            = scaleway_lb.main.id
  forward_protocol = "http"
  forward_port     = 80
  server_ips = [
    scaleway_instance_private_nic.web_0.private_ips[0].address,
    scaleway_instance_private_nic.web_1.private_ips[0].address,
  ]

  health_check_tcp {}
}

resource "scaleway_lb_frontend" "main" {
  name         = "http-frontend"
  lb_id        = scaleway_lb.main.id
  backend_id   = scaleway_lb_backend.main.id
  inbound_port = 80
}
`),
		"outputs.tf": []byte(incrementalOutputsTF(stage)),
	}
	if stage >= 2 {
		files["database.tf"] = []byte(`resource "scaleway_rdb_instance" "main" {
  name               = "incremental-project-paris-db"
  engine             = "PostgreSQL-15"
  node_type          = "DB-DEV-S"
  is_ha_cluster      = false
  disable_backup     = false
  volume_type        = "sbs_5k"
  volume_size_in_gb  = 10
  region             = var.region
  encryption_at_rest = true
  password           = var.db_password

  private_network {
    pn_id       = scaleway_vpc_private_network.main.id
    enable_ipam = true
  }
}

resource "scaleway_rdb_acl" "deny_public" {
  instance_id = scaleway_rdb_instance.main.id
  region      = var.region

  acl_rules {
    ip          = "10.0.0.0/24"
    description = "Allow private network only"
  }
}
`)
	}
	if stage >= 3 {
		files["redis.tf"] = []byte(`resource "scaleway_redis_cluster" "main" {
  name         = "incremental-project-paris-redis"
  version      = "7.0.12"
  node_type    = "RED1-MICRO"
  cluster_size = 1
  tls_enabled  = true
  user_name    = "redis-admin"
  password     = var.redis_password
  zone         = var.zone

  private_network {
    id = scaleway_vpc_private_network.main.id
  }
}
`)
	}
	return files
}

func incrementalVariablesTF(stage int) string {
	var builder strings.Builder
	builder.WriteString(`variable "region" {
  description = "Scaleway region"
  type        = string
  default     = "fr-par"
}

variable "zone" {
  description = "Scaleway zone"
  type        = string
  default     = "fr-par-1"
}
`)
	if stage >= 2 {
		builder.WriteString(`
variable "db_password" {
  description = "Password for the database instance"
  type        = string
  sensitive   = true
  default     = "changeme"
}
`)
	}
	if stage >= 3 {
		builder.WriteString(`
variable "redis_password" {
  description = "Password for the Redis cluster"
  type        = string
  sensitive   = true
  default     = "changeme-redis"
}
`)
	}
	return builder.String()
}

func incrementalOutputsTF(stage int) string {
	var builder strings.Builder
	builder.WriteString(`output "vpc_id" {
  description = "ID of the VPC"
  value       = scaleway_vpc.main.id
}

output "private_network_id" {
  description = "ID of the private network"
  value       = scaleway_vpc_private_network.main.id
}

output "web_0_public_ip" {
  description = "Public IP of web-0"
  value       = scaleway_instance_ip.web_0.address
}

output "web_1_public_ip" {
  description = "Public IP of web-1"
  value       = scaleway_instance_ip.web_1.address
}

output "lb_ip" {
  description = "Public IP of the load balancer"
  value       = scaleway_lb_ip.main.ip_address
}

output "lb_id" {
  description = "ID of the load balancer"
  value       = scaleway_lb.main.id
}
`)
	if stage >= 2 {
		builder.WriteString(`
output "db_instance_id" {
  description = "ID of the RDB instance"
  value       = scaleway_rdb_instance.main.id
}
`)
	}
	if stage >= 3 {
		builder.WriteString(`
output "redis_cluster_id" {
  description = "ID of the Redis cluster"
  value       = scaleway_redis_cluster.main.id
}
`)
	}
	return builder.String()
}

func startMockwayFromSource(t *testing.T) (string, func()) {
	t.Helper()

	port := pickFreePort(t)
	dbPath := filepath.Join(t.TempDir(), "mockway.sqlite")
	logPath := filepath.Join(t.TempDir(), "mockway.log")
	mockwayRoot := resolveSiblingMockwayRoot(t)
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create mockway log file: %v", err)
	}

	cmd := exec.Command("go", "run", "./cmd/mockway", "--port", fmt.Sprintf("%d", port), "--db", dbPath)
	cmd.Dir = mockwayRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start mockway: %v", err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitErr := waitForHTTP200(url + "/mock/state")
	if waitErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = logFile.Close()
		logPayload, _ := os.ReadFile(logPath)
		t.Fatalf("wait for mockway readiness: %v\nlog:\n%s", waitErr, string(logPayload))
	}

	cleanup := func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = logFile.Close()
	}
	return url, cleanup
}

func resolveSiblingMockwayRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "mockway"))
	if _, err := os.Stat(filepath.Join(root, "cmd", "mockway")); err != nil {
		t.Fatalf("resolve sibling mockway repo: %v", err)
	}
	return root
}

func waitForHTTP200(url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

func pickFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func fetchMockState(t *testing.T, baseURL string) map[string]any {
	t.Helper()
	resp, err := http.Get(baseURL + "/mock/state")
	if err != nil {
		t.Fatalf("get mock state: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected mock state status: %d", resp.StatusCode)
	}

	var state map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatalf("decode mock state: %v", err)
	}
	return state
}

func resourceIDs(state map[string]any, root, collection string) []string {
	rootMap, ok := state[root].(map[string]any)
	if !ok {
		return nil
	}
	items, ok := rootMap[collection].([]any)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func assertContains(t *testing.T, output, needle string) {
	t.Helper()
	if !strings.Contains(output, needle) {
		t.Fatalf("expected output to contain %q, got:\n%s", needle, output)
	}
}
