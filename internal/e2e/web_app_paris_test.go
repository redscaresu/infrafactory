package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_WebAppParis runs the canonical web-app-paris training scenario
// against a freshly-started mockway, using a stub generator that returns
// pre-baked HCL. It asserts the run reaches `target_reached`, which
// implies all topology, policy, and destruction criteria passed.
//
// Gated behind INFRAFACTORY_ENABLE_E2E=1 and requires `tofu` on PATH.
func TestE2E_WebAppParis(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}

	mock := StartMockway(t)

	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	scenarioPath := repoScenariosPath(t, "web-app-paris.yaml")

	WriteConfig(t, configPath, mock.URL, outputRoot)

	result := RunInfrafactory(t, InfrafactoryRunOptions{
		Args: []string{
			"run", scenarioPath,
			"--config", configPath,
		},
		GeneratorFiles: webAppParisFiles(),
	})

	if result.Err != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s\nstderr:\n%s\nmockway log: %s",
			result.Err, result.Stdout, result.Stderr, mock.LogPath())
	}
	for _, expected := range []string{
		"Status: success",
		"run/terminal_reason: pass (target_reached)",
	} {
		if !strings.Contains(result.Stdout, expected) {
			t.Fatalf("expected output to contain %q, got:\n%s", expected, result.Stdout)
		}
	}
}

// repoScenariosPath returns an absolute path to the canonical training
// scenario at <repo>/scenarios/training/<name>.
func repoScenariosPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(RepoRoot(t), "scenarios", "training", name)
}

// webAppParisFiles returns the pre-baked HCL that satisfies the
// web-app-paris scenario's acceptance criteria when applied through tofu
// against mockway:
//
//   - 2 small Scaleway instances (web servers) on private network only
//   - Public load balancer fronting them on port 80
//   - PostgreSQL RDB instance with encryption_at_rest, private-network ACL
//   - VPC + private network so compute can reach the database privately
//
// Mirrors the structure of the incremental stage-2 fixture in
// internal/cli/realtool_smoke_test.go but is owned by the e2e package
// since the cli helpers are unexported.
func webAppParisFiles() map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(webAppParisProvidersTF),
		"variables.tf": []byte(webAppParisVariablesTF),
		"network.tf":   []byte(webAppParisNetworkTF),
		"compute.tf":   []byte(webAppParisComputeTF),
		"database.tf":  []byte(webAppParisDatabaseTF),
		"loadbalancer.tf": []byte(webAppParisLoadBalancerTF),
		"outputs.tf":   []byte(webAppParisOutputsTF),
	}
}

const webAppParisProvidersTF = `terraform {
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
`

const webAppParisVariablesTF = `variable "region" {
  description = "Scaleway region"
  type        = string
  default     = "fr-par"
}

variable "zone" {
  description = "Scaleway zone"
  type        = string
  default     = "fr-par-1"
}

variable "db_password" {
  description = "Password for the database instance"
  type        = string
  sensitive   = true
  default     = "changeme"
}
`

const webAppParisNetworkTF = `resource "scaleway_vpc" "main" {
  name   = "web-app-paris-vpc"
  region = var.region
}

resource "scaleway_vpc_private_network" "main" {
  name   = "web-app-paris-pn"
  vpc_id = scaleway_vpc.main.id
  region = var.region

  ipv4_subnet {
    subnet = "10.0.0.0/24"
  }
}
`

const webAppParisComputeTF = `resource "scaleway_instance_ip" "web_0" {
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
`

const webAppParisDatabaseTF = `resource "scaleway_rdb_instance" "main" {
  name               = "web-app-paris-db"
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
`

const webAppParisLoadBalancerTF = `resource "scaleway_lb_ip" "main" {
  zone = var.zone
}

resource "scaleway_lb" "main" {
  name   = "web-app-paris-lb"
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
`

const webAppParisOutputsTF = `output "vpc_id" {
  description = "ID of the VPC"
  value       = scaleway_vpc.main.id
}

output "lb_ip" {
  description = "Public IP of the load balancer"
  value       = scaleway_lb_ip.main.ip_address
}

output "db_instance_id" {
  description = "ID of the RDB instance"
  value       = scaleway_rdb_instance.main.id
}
`
