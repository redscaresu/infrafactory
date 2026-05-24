package e2e

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runScalewayServiceScenario mirrors runGCPServiceScenario but for
// mockway-backed Scaleway scenarios. It writes infrafactory.yaml,
// runs `infrafactory run --no-destroy` with the supplied TF as the
// generator output, then asserts the post-apply mockway state contains
// each (root, collection) at minCount items.
//
// Pattern matches TestE2E_FullStackParis but extracted so each
// per-service test stays small.
func runScalewayServiceScenario(t *testing.T, mock *MockwayInstance, scenarioFile string, files map[string][]byte, expects []scalewayStateExpect) {
	t.Helper()

	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	scenarioPath := repoScenariosPath(t, scenarioFile)

	WriteConfig(t, configPath, mock.URL, outputRoot)

	noDestroy := RunInfrafactory(t, InfrafactoryRunOptions{
		Args:           []string{"run", scenarioPath, "--config", configPath, "--no-destroy"},
		GeneratorFiles: files,
	})
	if noDestroy.Err != nil {
		t.Fatalf("run --no-destroy failed: %v\nstdout:\n%s\nstderr:\n%s\nmockway log: %s",
			noDestroy.Err, noDestroy.Stdout, noDestroy.Stderr, mock.LogPath())
	}
	for _, want := range []string{"Status: success", "run/terminal_reason: pass (target_reached)"} {
		if !strings.Contains(noDestroy.Stdout, want) {
			t.Fatalf("expected first-run stdout to contain %q, got:\n%s", want, noDestroy.Stdout)
		}
	}

	state := mock.FetchState(t)
	for _, exp := range expects {
		// Some mockway collections (e.g., domain/dns_zones) identify
		// items by composite keys instead of `id`, so stateResourceIDs
		// returns nothing. Fall back to raw collection length.
		got := scalewayCollectionCount(state, exp.root, exp.collection)
		if got < exp.minCount {
			t.Errorf("expected at least %d %s/%s after apply, got %d",
				exp.minCount, exp.root, exp.collection, got)
		}
	}

	// Final destroy run cleans up and exercises the destruction
	// criterion.
	final := RunInfrafactory(t, InfrafactoryRunOptions{
		Args:           []string{"run", scenarioPath, "--config", configPath},
		GeneratorFiles: files,
	})
	if final.Err != nil {
		t.Fatalf("final run failed: %v\nstdout:\n%s", final.Err, final.Stdout)
	}
	if !strings.Contains(final.Stdout, "run/terminal_reason: pass (target_reached)") {
		t.Fatalf("expected final run to reach target_reached, got:\n%s", final.Stdout)
	}
}

type scalewayStateExpect struct {
	root       string
	collection string
	minCount   int
}

func scalewayCollectionCount(state map[string]any, root, collection string) int {
	rootMap, ok := state[root].(map[string]any)
	if !ok {
		return 0
	}
	items, ok := rootMap[collection].([]any)
	if !ok {
		return 0
	}
	return len(items)
}

// TestE2E_ScalewayRegistry — registry-paris scenario, exercises
// scaleway_registry_namespace against mockway's registry handlers.
func TestE2E_ScalewayRegistry(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartMockway(t)

	runScalewayServiceScenario(t, mock, "registry-paris.yaml",
		map[string][]byte{
			"providers.tf": []byte(scwProvidersTF),
			"main.tf":      []byte(scwRegistryTF),
		},
		[]scalewayStateExpect{
			{root: "registry", collection: "namespaces", minCount: 1},
		},
	)
}

// TestE2E_ScalewayRedis — redis-paris scenario, exercises
// scaleway_redis_cluster against mockway's redis handlers.
func TestE2E_ScalewayRedis(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartMockway(t)

	runScalewayServiceScenario(t, mock, "redis-paris.yaml",
		map[string][]byte{
			"providers.tf": []byte(scwProvidersTF),
			"main.tf":      []byte(scwRedisTF),
		},
		[]scalewayStateExpect{
			{root: "vpc", collection: "private_networks", minCount: 1},
			{root: "redis", collection: "clusters", minCount: 1},
		},
	)
}

// TestE2E_ScalewayIAM — iam-policies-paris scenario, exercises
// scaleway_iam_application + policy + api_key against mockway's iam
// handlers.
func TestE2E_ScalewayIAM(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartMockway(t)

	runScalewayServiceScenario(t, mock, "iam-policies-paris.yaml",
		map[string][]byte{
			"providers.tf": []byte(scwProvidersTF),
			"main.tf":      []byte(scwIAMTF),
		},
		[]scalewayStateExpect{
			{root: "iam", collection: "applications", minCount: 1},
			{root: "iam", collection: "policies", minCount: 1},
		},
	)
}

// TestE2E_ScalewayK8s — k8s-cluster-paris scenario, exercises
// scaleway_k8s_cluster + pool against mockway's k8s handlers.
func TestE2E_ScalewayK8s(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartMockway(t)

	runScalewayServiceScenario(t, mock, "k8s-cluster-paris.yaml",
		map[string][]byte{
			"providers.tf": []byte(scwProvidersTF),
			"main.tf":      []byte(scwK8sTF),
		},
		[]scalewayStateExpect{
			{root: "k8s", collection: "clusters", minCount: 1},
			{root: "k8s", collection: "pools", minCount: 1},
		},
	)
}

// TestE2E_ScalewayRDB — mysql-ha-paris scenario, exercises
// scaleway_rdb_instance against mockway's rdb handlers.
func TestE2E_ScalewayRDB(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartMockway(t)

	runScalewayServiceScenario(t, mock, "mysql-ha-paris.yaml",
		map[string][]byte{
			"providers.tf": []byte(scwProvidersTF),
			"main.tf":      []byte(scwRDBTF),
		},
		[]scalewayStateExpect{
			{root: "rdb", collection: "instances", minCount: 1},
		},
	)
}

// TestE2E_ScalewayBlock — block-paris scenario, exercises a
// standalone scaleway_block_volume against mockway's block handlers.
func TestE2E_ScalewayBlock(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartMockway(t)

	runScalewayServiceScenario(t, mock, "block-paris.yaml",
		map[string][]byte{
			"providers.tf": []byte(scwProvidersTF),
			"main.tf":      []byte(scwBlockTF),
		},
		[]scalewayStateExpect{
			{root: "block", collection: "volumes", minCount: 1},
		},
	)
}

// TestE2E_ScalewayDomain — domain-paris scenario, exercises
// scaleway_domain_zone + scaleway_domain_record against mockway's
// domain handlers.
func TestE2E_ScalewayDomain(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartMockway(t)

	runScalewayServiceScenario(t, mock, "domain-paris.yaml",
		map[string][]byte{
			"providers.tf": []byte(scwProvidersTF),
			"main.tf":      []byte(scwDomainTF),
		},
		[]scalewayStateExpect{
			{root: "domain", collection: "dns_zones", minCount: 1},
			{root: "domain", collection: "records", minCount: 1},
		},
	)
}

// IPAM e2e coverage deferred: a standalone scaleway_ipam_ip resource
// crashes the provider against mockway with "Plugin did not respond"
// during apply (provider-side bug surfaced by mockway's IPAM response
// shape — needs handler-side investigation). IPAM IPs are auto-managed
// by VPC + private_network usage so they're exercised indirectly by
// TestE2E_ScalewayRDB, ScalewayK8s, ScalewayRedis (all of which attach
// to a private network).

// TestE2E_ScalewayLB — lb-paris scenario, exercises a standalone
// scaleway_lb + frontend + backend against mockway's lb handlers.
func TestE2E_ScalewayLB(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartMockway(t)

	runScalewayServiceScenario(t, mock, "lb-paris.yaml",
		map[string][]byte{
			"providers.tf": []byte(scwProvidersTF),
			"main.tf":      []byte(scwLBTF),
		},
		[]scalewayStateExpect{
			{root: "lb", collection: "lbs", minCount: 1},
			{root: "lb", collection: "frontends", minCount: 1},
			{root: "lb", collection: "backends", minCount: 1},
		},
	)
}

// scwProvidersTF is the shared providers.tf for mockway-backed e2e
// tests. The Scaleway API URL is injected by infrafactory's test
// runner via SCW_API_URL so no per-resource endpoint config is needed.
const scwProvidersTF = `terraform {
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

const scwRegistryTF = `resource "scaleway_registry_namespace" "main" {
  name      = "svc-registry"
  region    = "fr-par"
  is_public = false
}
`

const scwRedisTF = `resource "scaleway_vpc" "main" {
  name   = "redis-vpc"
  region = "fr-par"
}

resource "scaleway_vpc_private_network" "main" {
  name   = "redis-pn"
  region = "fr-par"
  vpc_id = scaleway_vpc.main.id

  ipv4_subnet {
    subnet = "172.16.0.0/22"
  }
}

resource "scaleway_redis_cluster" "main" {
  name         = "svc-cache"
  version      = "7.0.12"
  node_type    = "RED1-MICRO"
  user_name    = "redis-admin"
  password     = "changeme123!"
  cluster_size = 1
  tls_enabled  = true
  zone         = "fr-par-1"

  private_network {
    id = scaleway_vpc_private_network.main.id
  }
}
`

const scwIAMTF = `resource "scaleway_iam_application" "svc" {
  name = "svc-application"
}

resource "scaleway_iam_policy" "svc" {
  name           = "svc-policy"
  application_id = scaleway_iam_application.svc.id

  rule {
    permission_set_names = ["AllProductsFullAccess"]
  }
}

resource "scaleway_iam_api_key" "svc" {
  application_id = scaleway_iam_application.svc.id
}
`

const scwK8sTF = `resource "scaleway_vpc" "main" {
  name   = "k8s-vpc"
  region = "fr-par"
}

resource "scaleway_vpc_private_network" "main" {
  name   = "k8s-pn"
  region = "fr-par"
  vpc_id = scaleway_vpc.main.id

  ipv4_subnet {
    subnet = "172.16.0.0/22"
  }
}

resource "scaleway_k8s_cluster" "main" {
  name                        = "svc-cluster"
  region                      = "fr-par"
  version                     = "1.30.2"
  cni                         = "cilium"
  delete_additional_resources = true
  private_network_id          = scaleway_vpc_private_network.main.id
}

resource "scaleway_k8s_pool" "main" {
  cluster_id  = scaleway_k8s_cluster.main.id
  region      = "fr-par"
  name        = "svc-pool"
  node_type   = "DEV1-M"
  size        = 1
  autoscaling = false
  autohealing = true
  zone        = "fr-par-1"
}
`

const scwBlockTF = `resource "scaleway_block_volume" "main" {
  name       = "svc-app-data"
  iops       = 5000
  size_in_gb = 10
  zone       = "fr-par-1"
}
`

const scwDomainTF = `resource "scaleway_domain_zone" "zone" {
  domain    = "example-test.com"
  subdomain = "app"
}

resource "scaleway_domain_record" "a" {
  dns_zone = "${scaleway_domain_zone.zone.subdomain}.${scaleway_domain_zone.zone.domain}"
  name     = "www"
  type     = "A"
  data     = "1.2.3.4"
  ttl      = 3600
}
`

const scwLBTF = `resource "scaleway_lb_ip" "main" {
  zone = "fr-par-1"
}

resource "scaleway_lb" "main" {
  name   = "svc-lb"
  ip_ids = [scaleway_lb_ip.main.id]
  zone   = "fr-par-1"
  type   = "LB-S"
}

resource "scaleway_lb_backend" "main" {
  lb_id            = scaleway_lb.main.id
  name             = "svc-be"
  forward_protocol = "http"
  forward_port     = 80
  proxy_protocol   = "none"
}

resource "scaleway_lb_frontend" "main" {
  lb_id        = scaleway_lb.main.id
  backend_id   = scaleway_lb_backend.main.id
  name         = "svc-fe"
  inbound_port = 80
}
`

const scwRDBTF = `resource "scaleway_vpc" "main" {
  name   = "rdb-vpc"
  region = "fr-par"
}

resource "scaleway_vpc_private_network" "main" {
  name   = "rdb-pn"
  region = "fr-par"
  vpc_id = scaleway_vpc.main.id

  ipv4_subnet {
    subnet = "172.16.0.0/22"
  }
}

resource "scaleway_instance_server" "app" {
  name  = "app-server"
  type  = "DEV1-S"
  zone  = "fr-par-1"
  image = "ubuntu_jammy"
}

resource "scaleway_instance_private_nic" "app" {
  server_id          = scaleway_instance_server.app.id
  private_network_id = scaleway_vpc_private_network.main.id
}

resource "scaleway_rdb_instance" "main" {
  name               = "svc-mysql"
  region             = "fr-par"
  node_type          = "DB-DEV-S"
  engine             = "MySQL-8"
  is_ha_cluster      = true
  disable_backup     = false
  user_name          = "app"
  password           = "ChangeMe123!"
  volume_type        = "sbs_5k"
  volume_size_in_gb  = 10
  encryption_at_rest = true

  private_network {
    pn_id       = scaleway_vpc_private_network.main.id
    enable_ipam = true
  }
}
`
