package e2e

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_GCPPubSub runs the gcp-pubsub training scenario end-to-end
// against a freshly-started fakegcp. The stub generator returns HCL
// that creates a topic + subscription, applies it through tofu, and
// the run loop's destroy phase tears it down. target_reached on a
// single iteration proves the topic→subscription FK chain plus the
// Layer-2 mock-deploy + destroy path against fakegcp.
//
// Gated behind INFRAFACTORY_ENABLE_E2E=1; requires `tofu` on PATH
// and the sibling ../fakegcp source repo.
func TestE2E_GCPPubSub(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	runGCPServiceScenario(t,
		mock,
		"gcp-pubsub.yaml",
		gcpPubSubFiles(mock.URL),
		[]gcpStateExpect{
			{root: "pubsub", collection: "topics", minCount: 1},
			{root: "pubsub", collection: "subscriptions", minCount: 1},
		},
	)
}

// TestE2E_GCPDNS runs the gcp-dns training scenario against fakegcp,
// proving the zone→record_set FK chain through the full apply +
// destroy lifecycle.
func TestE2E_GCPDNS(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	runGCPServiceScenario(t,
		mock,
		"gcp-dns.yaml",
		gcpDNSFiles(mock.URL),
		[]gcpStateExpect{
			{root: "dns", collection: "managed_zones", minCount: 1},
			{root: "dns", collection: "record_sets", minCount: 1},
		},
	)
}

// TestE2E_GCPCloudRun runs the gcp-cloud-run scenario end-to-end
// through fakegcp's Cloud Run v2 service handler set.
func TestE2E_GCPCloudRun(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	runGCPServiceScenario(t,
		mock,
		"gcp-cloud-run.yaml",
		gcpCloudRunFiles(mock.URL),
		[]gcpStateExpect{
			{root: "cloudrun", collection: "services", minCount: 1},
		},
	)
}

// TestE2E_GCPSecretManager runs the gcp-secret-manager scenario
// end-to-end through fakegcp, proving the secret→version FK chain.
func TestE2E_GCPSecretManager(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	runGCPServiceScenario(t,
		mock,
		"gcp-secret-manager.yaml",
		gcpSecretManagerFiles(mock.URL),
		[]gcpStateExpect{
			{root: "secretmanager", collection: "secrets", minCount: 1},
			{root: "secretmanager", collection: "versions", minCount: 1},
		},
	)
}

type gcpStateExpect struct {
	root       string
	collection string
	minCount   int
}

// runGCPServiceScenario centralises the two-stage run pattern shared
// by all four GCP-service e2e tests: --no-destroy first to capture
// post-apply state, then a destroy run to confirm orphan-free
// teardown. Both runs must end with target_reached.
func runGCPServiceScenario(t *testing.T, mock *MockwayInstance, scenarioFile string, files map[string][]byte, expects []gcpStateExpect) {
	t.Helper()

	workspace := t.TempDir()
	outputRoot := filepath.Join(workspace, "output")
	configPath := filepath.Join(workspace, "infrafactory.yaml")
	scenarioPath := repoScenariosPath(t, scenarioFile)

	// Pass an empty mockway URL — the loaded scenario's cloud is gcp,
	// so the cloudMockStateRouter dispatches to fakegcp; the mockway
	// fallback is never reached. WriteConfigMultiCloud requires both
	// args though, so use a placeholder.
	WriteConfigMultiCloud(t, configPath, "http://127.0.0.1:1", mock.URL, outputRoot)

	noDestroy := RunInfrafactory(t, InfrafactoryRunOptions{
		Args:           []string{"run", scenarioPath, "--config", configPath, "--no-destroy"},
		GeneratorFiles: files,
	})
	if noDestroy.Err != nil {
		t.Fatalf("run --no-destroy failed: %v\nstdout:\n%s\nstderr:\n%s\nfakegcp log: %s",
			noDestroy.Err, noDestroy.Stdout, noDestroy.Stderr, mock.LogPath())
	}
	for _, want := range []string{"Status: success", "run/terminal_reason: pass (target_reached)"} {
		if !strings.Contains(noDestroy.Stdout, want) {
			t.Fatalf("expected first-run stdout to contain %q, got:\n%s", want, noDestroy.Stdout)
		}
	}

	state := mock.FetchState(t)
	for _, exp := range expects {
		ids := stateResourceIDs(state, exp.root, exp.collection)
		if len(ids) < exp.minCount {
			t.Errorf("expected at least %d %s/%s after apply, got %d (ids=%v)",
				exp.minCount, exp.root, exp.collection, len(ids), ids)
		}
	}

	final := RunInfrafactory(t, InfrafactoryRunOptions{
		Args:           []string{"run", scenarioPath, "--config", configPath},
		GeneratorFiles: files,
	})
	if final.Err != nil {
		t.Fatalf("final destroy run failed: %v\nstdout:\n%s", final.Err, final.Stdout)
	}
	if !strings.Contains(final.Stdout, "run/terminal_reason: pass (target_reached)") {
		t.Fatalf("expected final destroy run to reach target_reached, got:\n%s", final.Stdout)
	}
}

// gcpProviderTF returns the providers.tf body that points all
// GCP-service custom endpoints at the running fakegcp instance. Each
// per-service test only uses the endpoint(s) it needs, but bundling
// them here keeps the four file maps short.
func gcpProviderTF(fakegcpURL string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 5.0"
    }
  }
}

provider "google" {
  project      = "fake-project"
  region       = "us-central1"
  zone         = "us-central1-a"
  access_token = "fake-token"

  batching {
    send_after = "0s"
  }

  pubsub_custom_endpoint         = "%s/v1/"
  dns_custom_endpoint            = "%s/dns/v1/"
  cloud_run_v2_custom_endpoint   = "%s/"
  secret_manager_custom_endpoint = "%s/"
}
`, fakegcpURL, fakegcpURL, fakegcpURL, fakegcpURL)
}

func gcpPubSubFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_pubsub_topic" "main" {
  name = "events"
}

resource "google_pubsub_subscription" "main" {
  name                 = "events-pull"
  topic                = google_pubsub_topic.main.name
  ack_deadline_seconds = 20
}
`),
	}
}

func gcpDNSFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_dns_managed_zone" "main" {
  name        = "test-zone"
  dns_name    = "test.example.invalid."
  description = "fakegcp e2e zone"
}

resource "google_dns_record_set" "main" {
  name         = "host.${google_dns_managed_zone.main.dns_name}"
  managed_zone = google_dns_managed_zone.main.name
  type         = "A"
  ttl          = 300
  rrdatas      = ["192.0.2.10"]
}
`),
	}
}

func gcpCloudRunFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_cloud_run_v2_service" "main" {
  name     = "api"
  location = "us-central1"

  template {
    containers {
      image = "us-docker.pkg.dev/cloudrun/container/hello"
    }
  }

  deletion_protection = false
}
`),
	}
}

func gcpSecretManagerFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_secret_manager_secret" "main" {
  secret_id = "db-credentials"

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "main" {
  secret      = google_secret_manager_secret.main.id
  secret_data = "fakegcp-test-payload"
}
`),
	}
}
