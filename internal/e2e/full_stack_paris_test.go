package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_FullStackParis runs the full-stack-paris training scenario,
// which exercises every Scaleway resource type infrafactory knows about:
// compute + VPC/private-network + RDB + Kubernetes + Redis + container
// registry + IAM (application/policy/api-key). It asserts target_reached
// and that mockway state contains every expected resource type after
// apply.
//
// Gated behind INFRAFACTORY_ENABLE_E2E=1 and requires `tofu` on PATH.
func TestE2E_FullStackParis(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}

	mock := StartMockway(t)

	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	scenarioPath := repoScenariosPath(t, "full-stack-paris.yaml")

	WriteConfig(t, configPath, mock.URL, outputRoot)

	// Capture state mid-run before infrafactory destroys it. We hook into
	// the generator stub so the generation request marks an apply boundary,
	// then poll mockway state after the run completes (state is destroyed)
	// — instead, run with --no-destroy so we can introspect resources, then
	// run a follow-up clean to destroy. Two-step pattern keeps the
	// assertion deterministic without racing the destroy stage.
	noDestroy := RunInfrafactory(t, InfrafactoryRunOptions{
		Args: []string{
			"run", scenarioPath,
			"--config", configPath,
			"--no-destroy",
		},
		GeneratorFiles: fullStackParisFiles(),
	})
	if noDestroy.Err != nil {
		t.Fatalf("run --no-destroy failed: %v\nstdout:\n%s\nstderr:\n%s\nmockway log: %s",
			noDestroy.Err, noDestroy.Stdout, noDestroy.Stderr, mock.LogPath())
	}
	for _, expected := range []string{
		"Status: success",
		"run/terminal_reason: pass (target_reached)",
	} {
		if !strings.Contains(noDestroy.Stdout, expected) {
			t.Fatalf("expected first run stdout to contain %q, got:\n%s", expected, noDestroy.Stdout)
		}
	}

	state := mock.FetchState(t)
	for _, tc := range []struct {
		root       string
		collection string
		minCount   int
	}{
		{root: "vpc", collection: "vpcs", minCount: 1},
		{root: "vpc", collection: "private_networks", minCount: 1},
		{root: "instance", collection: "servers", minCount: 1},
		{root: "rdb", collection: "instances", minCount: 1},
		{root: "k8s", collection: "clusters", minCount: 1},
		{root: "k8s", collection: "pools", minCount: 1},
		{root: "redis", collection: "clusters", minCount: 1},
		{root: "registry", collection: "namespaces", minCount: 1},
		{root: "iam", collection: "applications", minCount: 1},
	} {
		ids := stateResourceIDs(state, tc.root, tc.collection)
		if len(ids) < tc.minCount {
			t.Errorf("expected at least %d %s/%s after apply, got %d (ids=%v)",
				tc.minCount, tc.root, tc.collection, len(ids), ids)
		}
	}

	// Final destroy run cleans up. Asserting target_reached on the destroy
	// pass also exercises the destruction acceptance criterion in the
	// scenario (no_orphans).
	final := RunInfrafactory(t, InfrafactoryRunOptions{
		Args: []string{
			"run", scenarioPath,
			"--config", configPath,
		},
		GeneratorFiles: fullStackParisFiles(),
	})
	if final.Err != nil {
		t.Fatalf("final run failed: %v\nstdout:\n%s", final.Err, final.Stdout)
	}
	if !strings.Contains(final.Stdout, "run/terminal_reason: pass (target_reached)") {
		t.Fatalf("expected final run to reach target_reached, got:\n%s", final.Stdout)
	}
}

// stateResourceIDs walks mockway's state JSON and returns the IDs of
// resources at state[root][collection]. Returns nil when the path is
// absent (mockway omits empty collections).
func stateResourceIDs(state map[string]any, root, collection string) []string {
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

func fullStackParisFiles() map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(fullStackParisProvidersTF),
		"variables.tf": []byte(fullStackParisVariablesTF),
		"network.tf":   []byte(fullStackParisNetworkTF),
		"security.tf":  []byte(fullStackParisSecurityTF),
		"compute.tf":   []byte(fullStackParisComputeTF),
		"database.tf":  []byte(fullStackParisDatabaseTF),
		"kubernetes.tf": []byte(fullStackParisKubernetesTF),
		"redis.tf":     []byte(fullStackParisRedisTF),
		"registry.tf":  []byte(fullStackParisRegistryTF),
		"iam.tf":       []byte(fullStackParisIAMTF),
		"outputs.tf":   []byte(fullStackParisOutputsTF),
	}
}

const fullStackParisProvidersTF = `terraform {
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

const fullStackParisVariablesTF = `variable "redis_password" {
  description = "Password for the Redis cluster"
  type        = string
  default     = "changeme"
  sensitive   = true
}
`

const fullStackParisNetworkTF = `resource "scaleway_vpc" "main" {
  name   = "full-stack-vpc"
  region = "fr-par"
}

resource "scaleway_vpc_private_network" "main" {
  name   = "full-stack-pn"
  region = "fr-par"
  vpc_id = scaleway_vpc.main.id

  ipv4_subnet {
    subnet = "172.16.0.0/22"
  }
}
`

const fullStackParisSecurityTF = `resource "scaleway_instance_security_group" "web" {
  name                    = "web-sg"
  zone                    = "fr-par-1"
  inbound_default_policy  = "drop"
  outbound_default_policy = "accept"

  inbound_rule {
    action   = "accept"
    port     = 80
    protocol = "TCP"
  }

  inbound_rule {
    action   = "accept"
    port     = 443
    protocol = "TCP"
  }

  inbound_rule {
    action   = "accept"
    port     = 22
    protocol = "TCP"
  }
}
`

// full-stack-paris has no load balancer or public ingress in its
// scenario definition — the web server lives on the private network only.
// Avoiding a public scaleway_instance_ip also keeps the static
// no_public_endpoints policy green on incremental re-runs (the policy
// matches IPs whose server binding becomes non-null in state).
const fullStackParisComputeTF = `resource "scaleway_instance_server" "web" {
  name              = "web-server"
  type              = "DEV1-S"
  zone              = "fr-par-1"
  image             = "ubuntu_jammy"
  security_group_id = scaleway_instance_security_group.web.id
}

resource "scaleway_instance_private_nic" "web" {
  server_id          = scaleway_instance_server.web.id
  private_network_id = scaleway_vpc_private_network.main.id
}
`

const fullStackParisDatabaseTF = `resource "scaleway_rdb_instance" "main" {
  name               = "full-stack-db"
  node_type          = "DB-DEV-S"
  engine             = "PostgreSQL-15"
  is_ha_cluster      = false
  disable_backup     = false
  volume_type        = "sbs_5k"
  volume_size_in_gb  = 10
  region             = "fr-par"
  encryption_at_rest = true

  private_network {
    pn_id       = scaleway_vpc_private_network.main.id
    enable_ipam = true
  }
}
`

const fullStackParisKubernetesTF = `resource "scaleway_k8s_cluster" "main" {
  name                        = "full-stack-k8s"
  version                     = "1.31.0"
  cni                         = "cilium"
  region                      = "fr-par"
  private_network_id          = scaleway_vpc_private_network.main.id
  delete_additional_resources = true
}

resource "scaleway_k8s_pool" "main" {
  name        = "default-pool"
  cluster_id  = scaleway_k8s_cluster.main.id
  node_type   = "DEV1-M"
  size        = 1
  min_size    = 1
  max_size    = 3
  autoscaling = true
  autohealing = true
  zone        = "fr-par-1"
}
`

const fullStackParisRedisTF = `resource "scaleway_redis_cluster" "main" {
  name         = "full-stack-cache"
  version      = "7.0.12"
  node_type    = "RED1-MICRO"
  user_name    = "redis-admin"
  password     = var.redis_password
  cluster_size = 1
  tls_enabled  = true
  zone         = "fr-par-1"

  private_network {
    id = scaleway_vpc_private_network.main.id
  }
}
`

const fullStackParisRegistryTF = `resource "scaleway_registry_namespace" "main" {
  name      = "full-stack-registry"
  region    = "fr-par"
  is_public = false
}
`

const fullStackParisIAMTF = `resource "scaleway_iam_application" "cicd" {
  name = "cicd-application"
}

resource "scaleway_iam_policy" "cicd" {
  name           = "cicd-policy"
  application_id = scaleway_iam_application.cicd.id

  rule {
    permission_set_names = ["AllProductsFullAccess"]
  }
}

resource "scaleway_iam_api_key" "cicd" {
  application_id = scaleway_iam_application.cicd.id
  description    = "CI/CD pipeline API key"
}
`

const fullStackParisOutputsTF = `output "vpc_id" {
  description = "ID of the VPC"
  value       = scaleway_vpc.main.id
}

output "private_network_id" {
  description = "ID of the private network"
  value       = scaleway_vpc_private_network.main.id
}

output "web_server_id" {
  value = scaleway_instance_server.web.id
}

output "web_private_nic_id" {
  value = scaleway_instance_private_nic.web.id
}

output "database_id" {
  value = scaleway_rdb_instance.main.id
}

output "k8s_cluster_id" {
  value = scaleway_k8s_cluster.main.id
}

output "redis_cluster_id" {
  value = scaleway_redis_cluster.main.id
}

output "registry_endpoint" {
  value = scaleway_registry_namespace.main.endpoint
}

output "iam_application_id" {
  value = scaleway_iam_application.cicd.id
}
`
