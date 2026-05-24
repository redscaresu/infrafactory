package e2e

import (
	"os/exec"
	"testing"
)

// TestE2E_GCPFullStack runs the gcp-full-stack training scenario,
// which exercises the GCP composition infrafactory cares about against
// fakegcp: VPC + private subnet (compute networks), GKE cluster +
// node pool, private Cloud SQL Postgres, Memorystore Redis, and GCS
// storage bucket. Mirrors the Scaleway full-stack composition test
// (TestE2E_FullStackParis) — same lifecycle contract, different cloud.
//
// IAM service account is in the scenario YAML but NOT in this
// composition test's HCL because of a provider-version conflict:
// `google_container_node_pool` reliably succeeds against fakegcp only
// on provider-google >= v6, but `google_service_account` requires
// v5 (provider-google v6+ stopped honoring iam_custom_endpoint for
// the iam.admin.v1 API path). A single root module can only pin one
// provider version. IAM is exercised in isolation by TestE2E_GCPIAM
// against the v5 line.
//
// Gated behind INFRAFACTORY_ENABLE_E2E=1 and requires `tofu` on PATH.
func TestE2E_GCPFullStack(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	runGCPServiceScenario(t,
		mock,
		"gcp-full-stack.yaml",
		gcpFullStackFiles(mock.URL),
		[]gcpStateExpect{
			{root: "compute", collection: "networks", minCount: 1},
			{root: "compute", collection: "subnetworks", minCount: 1},
			{root: "container", collection: "clusters", minCount: 1},
			{root: "container", collection: "nodePools", minCount: 1},
			{root: "sql", collection: "instances", minCount: 1},
			{root: "sql", collection: "databases", minCount: 1},
			{root: "storage", collection: "buckets", minCount: 1},
			{root: "redis", collection: "instances", minCount: 1},
		},
		nil,
	)
}

// gcpFullStackFiles composes the multi-service HCL the gcp-full-stack
// scenario expects. Every resource declares region/zone explicitly
// because the region_restriction policy reads them from state
// (provider-default `region` is fallback only).
func gcpFullStackFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"network.tf": []byte(`resource "google_compute_network" "main" {
  name                    = "fs-net"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "main" {
  name          = "fs-subnet"
  ip_cidr_range = "10.50.0.0/24"
  region        = "europe-west1"
  network       = google_compute_network.main.id
}

resource "google_compute_global_address" "sql_peering" {
  name          = "fs-sql-peering"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.main.id
}
`),
		"gke.tf": []byte(`resource "google_container_cluster" "main" {
  name     = "fs-cluster"
  location = "europe-west1"

  network    = "fs-net"
  subnetwork = "fs-subnet"

  remove_default_node_pool = true
  initial_node_count       = 1

  deletion_protection = false

  depends_on = [google_compute_subnetwork.main]
}

resource "google_container_node_pool" "primary" {
  name       = "fs-primary-pool"
  location   = "europe-west1"
  cluster    = google_container_cluster.main.name
  node_count = 1

  node_config {
    machine_type = "e2-small"
  }
}
`),
		"sql.tf": []byte(`resource "google_sql_database_instance" "main" {
  name             = "fs-sql"
  database_version = "POSTGRES_15"
  region           = "europe-west1"

  encryption_key_name = "projects/fake-project/locations/europe-west1/keyRings/sql/cryptoKeys/sql-key"

  settings {
    tier = "db-f1-micro"

    ip_configuration {
      ipv4_enabled    = false
      private_network = google_compute_network.main.id
    }
  }

  deletion_protection = false
}

resource "google_sql_database" "main" {
  name     = "app"
  instance = google_sql_database_instance.main.name
}
`),
		"storage.tf": []byte(`resource "google_storage_bucket" "assets" {
  name     = "fs-assets-bucket"
  location = "europe-west1"

  encryption {
    default_kms_key_name = "projects/fake-project/locations/europe-west1/keyRings/state/cryptoKeys/state-key"
  }

  force_destroy = true
}
`),
		"redis.tf": []byte(`resource "google_redis_instance" "cache" {
  name           = "fs-cache"
  tier           = "BASIC"
  memory_size_gb = 1
  region         = "europe-west1"
  redis_version  = "REDIS_7_0"
}
`),
	}
}
