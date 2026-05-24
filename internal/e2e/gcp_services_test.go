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
		&gcpUpdate{
			files: gcpPubSubUpdateFiles(mock.URL),
			verify: func(t *testing.T, state map[string]any) {
				ack := firstSubscriptionAckDeadline(state)
				if ack != 45 {
					t.Errorf("expected subscription ackDeadlineSeconds=45 after update, got %v", ack)
				}
			},
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
			{root: "dns", collection: "zones", minCount: 1},
			{root: "dns", collection: "record_sets", minCount: 1},
		},
		&gcpUpdate{
			files: gcpDNSUpdateFiles(mock.URL),
			verify: func(t *testing.T, state map[string]any) {
				ttl := firstRecordSetTTL(state)
				if ttl != 600 {
					t.Errorf("expected record-set ttl=600 after update, got %v", ttl)
				}
			},
			// Cloud DNS record sets are mutated through the v1
			// transactional changes API as a delete + add of the
			// owner+type pair. There is no in-place rrset patch in
			// the API surface, so a fresh creationTime is the
			// correct, expected outcome of an "update".
			allowReplaceCollections: []string{"dns/record_sets"},
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
		&gcpUpdate{
			files: gcpCloudRunUpdateFiles(mock.URL),
			verify: func(t *testing.T, state map[string]any) {
				labels := firstCloudRunServiceLabels(state)
				if labels["env"] != "staging" {
					t.Errorf("expected cloud-run service label env=staging after update, got %v", labels)
				}
			},
		},
	)
}

// TestE2E_GCPLoadBalancer drives the global external HTTPS LB stack
// (forwarding rule → HTTPS proxy → URL map → backend service →
// health check, plus SSL cert + global address) through fakegcp's
// load-balancer handlers end-to-end.
func TestE2E_GCPLoadBalancer(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	runGCPServiceScenario(t,
		mock,
		"gcp-load-balancer.yaml",
		gcpLoadBalancerFiles(mock.URL),
		[]gcpStateExpect{
			{root: "lb", collection: "global_addresses", minCount: 1},
			{root: "lb", collection: "health_checks", minCount: 1},
			{root: "lb", collection: "backend_services", minCount: 1},
			{root: "lb", collection: "url_maps", minCount: 1},
			{root: "lb", collection: "ssl_certificates", minCount: 1},
			{root: "lb", collection: "target_https_proxies", minCount: 1},
			{root: "lb", collection: "global_forwarding_rules", minCount: 1},
		},
		&gcpUpdate{
			files: gcpLoadBalancerUpdateFiles(mock.URL),
			verify: func(t *testing.T, state map[string]any) {
				desc := firstBackendServiceDescription(state)
				if desc != "updated-backend" {
					t.Errorf("expected backend-service description=updated-backend after update, got %q", desc)
				}
			},
		},
	)
}

// TestE2E_GCPIAM provisions a service account + key against fakegcp's
// IAM handler set.
func TestE2E_GCPIAM(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	runGCPServiceScenario(t,
		mock,
		"gcp-iam.yaml",
		gcpIAMFiles(mock.URL),
		[]gcpStateExpect{
			{root: "iam", collection: "serviceAccounts", minCount: 1},
			{root: "iam", collection: "keys", minCount: 1},
		},
		&gcpUpdate{
			files: gcpIAMUpdateFiles(mock.URL),
			verify: func(t *testing.T, state map[string]any) {
				display := firstServiceAccountDisplayName(state)
				if display != "CI runner (rotated)" {
					t.Errorf("expected service account displayName='CI runner (rotated)' after update, got %q", display)
				}
			},
			// google_service_account_key regenerates on every refresh by
			// design; the parent service account is what we're actually
			// patching here, and the identity check still asserts that
			// stays in place.
			allowReplaceCollections: []string{"iam/keys"},
		},
	)
}

// TestE2E_GCPStorage provisions a Cloud Storage bucket against
// fakegcp's storage handlers.
func TestE2E_GCPStorage(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	runGCPServiceScenario(t,
		mock,
		"gcp-storage.yaml",
		gcpStorageFiles(mock.URL),
		[]gcpStateExpect{
			{root: "storage", collection: "buckets", minCount: 1},
		},
		&gcpUpdate{
			files: gcpStorageUpdateFiles(mock.URL),
			verify: func(t *testing.T, state map[string]any) {
				labels := firstBucketLabels(state)
				if labels["env"] != "prod" {
					t.Errorf("expected bucket label env=prod after update, got %v", labels)
				}
			},
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
		&gcpUpdate{
			files: gcpSecretManagerUpdateFiles(mock.URL),
			verify: func(t *testing.T, state map[string]any) {
				labels := firstSecretLabels(state)
				if labels["rotation"] != "monthly" {
					t.Errorf("expected secret label rotation=monthly after update, got %v", labels)
				}
			},
		},
	)
}

// TestE2E_GCPCompute provisions a VPC + subnet + firewall + VM against
// fakegcp's compute handlers (gcp-vm-network scenario, europe-west1).
func TestE2E_GCPCompute(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	runGCPServiceScenario(t,
		mock,
		"gcp-vm-network.yaml",
		gcpComputeFiles(mock.URL),
		[]gcpStateExpect{
			{root: "compute", collection: "networks", minCount: 1},
			{root: "compute", collection: "subnetworks", minCount: 1},
			{root: "compute", collection: "instances", minCount: 1},
		},
		nil,
	)
}

// TestE2E_GCPCloudSQL provisions a private PostgreSQL Cloud SQL
// instance + VPC peering + an api-server VM against fakegcp's
// sql / compute handlers (gcp-cloud-sql scenario, europe-west1).
func TestE2E_GCPCloudSQL(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	runGCPServiceScenario(t,
		mock,
		"gcp-cloud-sql.yaml",
		gcpCloudSQLFiles(mock.URL),
		[]gcpStateExpect{
			{root: "sql", collection: "instances", minCount: 1},
			{root: "sql", collection: "databases", minCount: 1},
			{root: "compute", collection: "instances", minCount: 1},
		},
		nil,
	)
}

// TestE2E_GCPGKE provisions a GKE cluster + node pool + CMEK-encrypted
// state bucket against fakegcp's container + storage handlers
// (gcp-gke-cluster scenario, europe-west1).
func TestE2E_GCPGKE(t *testing.T) {
	SkipUnlessEnabled(t)
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("tofu binary required for e2e: %v", err)
	}
	mock := StartFakegcp(t)

	runGCPServiceScenario(t,
		mock,
		"gcp-gke-cluster.yaml",
		gcpGKEFiles(mock.URL),
		[]gcpStateExpect{
			{root: "container", collection: "clusters", minCount: 1},
			{root: "container", collection: "nodePools", minCount: 1},
			{root: "storage", collection: "buckets", minCount: 1},
		},
		nil,
	)
}

type gcpStateExpect struct {
	root       string
	collection string
	minCount   int
}

// gcpUpdate carries the second-stage HCL plus a post-update assertion.
// The update phase swaps the original GeneratorFiles for updateFiles
// and re-runs --no-destroy; the resulting fakegcp state is then
// inspected by verify. This proves the resource's Update path works
// (otherwise tofu apply would fail or the expected change wouldn't
// surface).
//
// allowReplaceCollections lists root/collection pairs that are
// allowed to be replaced rather than patched in place. The default
// is empty: every resource is expected to be patched in place.
// Two cases legitimately need this opt-out today:
//   - dns/record_sets: Cloud DNS has no in-place rrset patch in its
//     API surface — mutations go through the v1 transactional changes
//     API as a delete + add of the (name, type) pair, so a fresh
//     creationTime on the addition side is the expected outcome.
//   - iam/keys: google_service_account_key regenerates on every
//     refresh by design (see the provider's `keepers` documentation).
// Even when a collection is in this list the harness still asserts a
// stable logical-key set (and a per-SA index for iam/keys) so a wipe
// or unexpected migration is not silently accepted.
type gcpUpdate struct {
	files                   map[string][]byte
	verify                  func(t *testing.T, state map[string]any)
	allowReplaceCollections []string
}

// gcpStateItemCount returns len(state[root][collection]). fakegcp's
// FullState wraps each service in a per-service map of slice-valued
// collections (pubsub→{topics,subscriptions}, dns→{zones,record_sets},
// cloudrun→{services}, secretmanager→{secrets,versions}, …). Items
// inside use GCP-shaped fields (name, not id), so we just count.
func gcpStateItemCount(state map[string]any, root, collection string) int {
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

// runGCPServiceScenario drives the full Create → (optional) Update →
// Delete lifecycle for a GCP-service scenario:
//   1. --no-destroy with `files`         → proves Create + post-apply state.
//   2. --no-destroy with `update.files`  → proves Update (only if update != nil).
//   3. final run with `update.files`     → proves Delete via orphan-free teardown.
// Every stage must end with target_reached. The update stage's verify
// callback inspects fakegcp state to confirm the mutation actually
// surfaced (otherwise an "update" might silently be a recreate or a
// no-op).
func runGCPServiceScenario(t *testing.T, mock *MockwayInstance, scenarioFile string, files map[string][]byte, expects []gcpStateExpect, update *gcpUpdate) {
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

	stateAfterCreate := mock.FetchState(t)
	for _, exp := range expects {
		got := gcpStateItemCount(stateAfterCreate, exp.root, exp.collection)
		if got < exp.minCount {
			t.Errorf("expected at least %d %s/%s after apply, got %d",
				exp.minCount, exp.root, exp.collection, got)
		}
	}

	finalFiles := files
	if update != nil {
		// Snapshot every collection's fakegcp-assigned `name` (which
		// includes the immutable id segment for resources that don't
		// expose `id`) so we can detect a delete/recreate rather than
		// a real in-place update. CreateRun uses provider-assigned ids;
		// if the update phase silently destroys + recreates, the names
		// would change.
		identitiesBefore := collectIdentities(stateAfterCreate, expects)

		updateRun := RunInfrafactory(t, InfrafactoryRunOptions{
			Args:           []string{"run", scenarioPath, "--config", configPath, "--no-destroy"},
			GeneratorFiles: update.files,
		})
		if updateRun.Err != nil {
			t.Fatalf("update --no-destroy run failed: %v\nstdout:\n%s\nstderr:\n%s\nfakegcp log: %s",
				updateRun.Err, updateRun.Stdout, updateRun.Stderr, mock.LogPath())
		}
		if !strings.Contains(updateRun.Stdout, "run/terminal_reason: pass (target_reached)") {
			t.Fatalf("expected update run to reach target_reached, got:\n%s", updateRun.Stdout)
		}
		stateAfterUpdate := mock.FetchState(t)
		identitiesAfter := collectIdentities(stateAfterUpdate, expects)
		allowReplace := map[string]struct{}{}
		for _, k := range update.allowReplaceCollections {
			allowReplace[k] = struct{}{}
		}
		for key, before := range identitiesBefore {
			if _, exempt := allowReplace[key]; exempt {
				// Even when replace-on-update is the documented API
				// behaviour for this collection, the *logical key set*
				// must survive — an unexpected wipe or migration to a
				// different parent (e.g. an iam/keys recreate that
				// silently rebound to a different service account)
				// would slip through the identity check otherwise.
				lb := collectReplaceLogicalKeys(stateAfterCreate, key)
				la := collectReplaceLogicalKeys(stateAfterUpdate, key)
				if !sameIdentities(lb, la) {
					t.Errorf("update phase changed the logical %s set (replace allowed but contents differ): before=%v after=%v",
						key, lb, la)
				}
				continue
			}
			after := identitiesAfter[key]
			if !sameIdentities(before, after) {
				t.Errorf("update phase appears to have replaced (not patched) %s: ids before=%v after=%v",
					key, before, after)
			}
		}
		if update.verify != nil {
			update.verify(t, stateAfterUpdate)
		}
		finalFiles = update.files
	}

	final := RunInfrafactory(t, InfrafactoryRunOptions{
		Args:           []string{"run", scenarioPath, "--config", configPath},
		GeneratorFiles: finalFiles,
	})
	if final.Err != nil {
		t.Fatalf("final destroy run failed: %v\nstdout:\n%s", final.Err, final.Stdout)
	}
	if !strings.Contains(final.Stdout, "run/terminal_reason: pass (target_reached)") {
		t.Fatalf("expected final destroy run to reach target_reached, got:\n%s", final.Stdout)
	}
}

// gcpProviderTF returns the providers.tf body that points every
// GCP custom endpoint we exercise in e2e tests at the running fakegcp
// instance. Per-service tests only use the endpoint(s) they need, but
// bundling them here keeps each file map short.
//
// version pin: `>= 5.0` for most resources (v5+v6 both work). IAM
// service-account ops need `~> 5.0` because provider-google v6+
// stopped honoring `iam_custom_endpoint` for the `iam.admin.v1` API
// path used by `google_service_account`; IAM tests use
// gcpProviderTFV5 instead.
func gcpProviderTF(fakegcpURL string) string {
	return gcpProviderTFPinned(fakegcpURL, ">= 5.0")
}

// gcpProviderTFV5 pins provider-google to the v5 line. Required for
// `google_service_account` resources because v6+ bypasses
// `iam_custom_endpoint` for service-account ops, hitting real
// iam.googleapis.com with a fake token (M41 / M46).
func gcpProviderTFV5(fakegcpURL string) string {
	return gcpProviderTFPinned(fakegcpURL, "~> 5.0")
}

func gcpProviderTFPinned(fakegcpURL, version string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = %[2]q
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

  # Endpoint shape per terraform-provider-google: the provider PREPENDS
  # the service's full API path (e.g. "/v1/projects/...", "/sql/v1beta4/
  # projects/...") to the configured *_custom_endpoint. So:
  #   - iam, container, pubsub, secret_manager, cloud_resource_manager:
  #     provider prepends /v1/... → endpoint MUST be the host root (no
  #     /v1/ suffix) or routes double-prefix to /v1/v1/...
  #   - compute, storage, sql, dns, cloud_run_v2: provider prepends only
  #     the resource path (e.g. /projects/...), so the endpoint MUST
  #     include the service API prefix.
  # M41/M46 closeout — pre-S53 these were inconsistent; iam routed to
  # /v1/v1/... and traffic actually hit real Google instead of fakegcp.
  compute_custom_endpoint                = "%[1]s/compute/v1/"
  iam_custom_endpoint                    = "%[1]s/"
  cloud_resource_manager_custom_endpoint = "%[1]s/"
  storage_custom_endpoint                = "%[1]s/storage/v1/"
  pubsub_custom_endpoint                 = "%[1]s/v1/"
  dns_custom_endpoint                    = "%[1]s/dns/v1/"
  cloud_run_v2_custom_endpoint           = "%[1]s/v2/"
  secret_manager_custom_endpoint         = "%[1]s/v1/"
  sql_custom_endpoint                    = "%[1]s/sql/v1beta4/"
  container_custom_endpoint              = "%[1]s/"
}
`, fakegcpURL, version)
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

func gcpLoadBalancerFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_compute_global_address" "lb" {
  name = "lb-ip"
}

resource "google_compute_health_check" "hc" {
  name = "lb-hc"

  http_health_check {
    port         = 80
    request_path = "/"
  }
}

resource "google_compute_backend_service" "be" {
  name          = "lb-be"
  protocol      = "HTTP"
  health_checks = [google_compute_health_check.hc.id]
}

resource "google_compute_url_map" "um" {
  name            = "lb-um"
  default_service = google_compute_backend_service.be.id
}

resource "google_compute_ssl_certificate" "cert" {
  name        = "lb-cert"
  private_key = "fake-private-key"
  certificate = "fake-certificate"
}

resource "google_compute_target_https_proxy" "thp" {
  name             = "lb-thp"
  url_map          = google_compute_url_map.um.id
  ssl_certificates = [google_compute_ssl_certificate.cert.id]
}

resource "google_compute_global_forwarding_rule" "fr" {
  name       = "lb-fr"
  ip_address = google_compute_global_address.lb.id
  port_range = "443"
  target     = google_compute_target_https_proxy.thp.id
}
`),
	}
}

func gcpIAMFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTFV5(fakegcpURL)),
		"main.tf": []byte(`resource "google_service_account" "ci" {
  account_id   = "ci-runner"
  display_name = "CI runner service account"
}

resource "google_service_account_key" "ci" {
  service_account_id = google_service_account.ci.name
  key_algorithm      = "KEY_ALG_RSA_2048"
}
`),
	}
}

func gcpStorageFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_storage_bucket" "assets" {
  name          = "fake-project-app-assets"
  location      = "us-central1"
  force_destroy = true

  uniform_bucket_level_access = true

  encryption {
    default_kms_key_name = "projects/fake-project/locations/us-central1/keyRings/r/cryptoKeys/k"
  }
}
`),
	}
}

// gcpComputeFiles satisfies gcp-vm-network: VPC + subnet + firewall +
// VM all in europe-west1. region_restriction policy reads each
// resource's region/zone so europe-west1 must be explicit on every
// resource (provider-default `region` is fallback only).
func gcpComputeFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_compute_network" "main" {
  name                    = "vm-net"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "main" {
  name          = "vm-subnet"
  ip_cidr_range = "10.20.0.0/24"
  region        = "europe-west1"
  network       = google_compute_network.main.id
}

resource "google_compute_firewall" "allow_ssh" {
  name    = "vm-allow-ssh"
  network = google_compute_network.main.name

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }

  source_ranges = ["198.51.100.0/24"]
}

resource "google_compute_instance" "main" {
  name         = "web-vm"
  machine_type = "e2-small"
  zone         = "europe-west1-b"

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-11"
    }
  }

  network_interface {
    network    = "vm-net"
    subnetwork = "vm-subnet"
  }

  depends_on = [google_compute_subnetwork.main]
}
`),
	}
}

// gcpCloudSQLFiles satisfies gcp-cloud-sql: private Postgres + VPC
// peering + api-server VM in europe-west1 with CMEK on the SQL
// instance (encryption_at_rest) and ipv4_enabled=false +
// authorized_networks empty (no_public_endpoints / public blocked).
func gcpCloudSQLFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_compute_network" "main" {
  name                    = "sql-net"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "main" {
  name          = "sql-subnet"
  ip_cidr_range = "10.30.0.0/24"
  region        = "europe-west1"
  network       = google_compute_network.main.id
}

resource "google_compute_global_address" "private_ip" {
  name          = "sql-private-ip"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.main.id
}

resource "google_sql_database_instance" "main" {
  name             = "private-sql"
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

resource "google_compute_instance" "api" {
  name         = "api-server"
  machine_type = "e2-small"
  zone         = "europe-west1-b"

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-11"
    }
  }

  network_interface {
    network    = "sql-net"
    subnetwork = "sql-subnet"
  }

  depends_on = [google_compute_subnetwork.main]
}
`),
	}
}

// gcpGKEFiles satisfies gcp-gke-cluster: GKE cluster + node pool +
// CMEK-encrypted state bucket (encryption_at_rest reads
// bucket.encryption.default_kms_key_name) in europe-west1.
func gcpGKEFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_compute_network" "gke" {
  name                    = "gke-net"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "gke" {
  name          = "gke-subnet"
  ip_cidr_range = "10.40.0.0/24"
  region        = "europe-west1"
  network       = google_compute_network.gke.id
}

resource "google_container_cluster" "main" {
  name     = "tf-cluster"
  location = "europe-west1"

  network    = "gke-net"
  subnetwork = "gke-subnet"

  remove_default_node_pool = true
  initial_node_count       = 1

  deletion_protection = false

  depends_on = [google_compute_subnetwork.gke]
}

resource "google_container_node_pool" "primary" {
  name       = "primary-pool"
  location   = "europe-west1"
  cluster    = google_container_cluster.main.name
  node_count = 1

  node_config {
    machine_type = "e2-small"
  }
}

resource "google_storage_bucket" "state" {
  name     = "tf-cluster-state-bucket"
  location = "europe-west1"

  encryption {
    default_kms_key_name = "projects/fake-project/locations/europe-west1/keyRings/state/cryptoKeys/state-key"
  }

  force_destroy = true
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

// Update-stage HCL: each *UpdateFiles maps the same resources as
// the corresponding *Files but with at least one mutable field
// changed, so re-applying exercises the resource's Update path
// rather than its Create path.

func gcpPubSubUpdateFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_pubsub_topic" "main" {
  name = "events"
}

resource "google_pubsub_subscription" "main" {
  name                 = "events-pull"
  topic                = google_pubsub_topic.main.name
  ack_deadline_seconds = 45
}
`),
	}
}

func gcpDNSUpdateFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_dns_managed_zone" "main" {
  name        = "test-zone"
  dns_name    = "test.example.invalid."
  description = "fakegcp e2e zone (updated)"
}

resource "google_dns_record_set" "main" {
  name         = "host.${google_dns_managed_zone.main.dns_name}"
  managed_zone = google_dns_managed_zone.main.name
  type         = "A"
  ttl          = 600
  rrdatas      = ["192.0.2.10"]
}
`),
	}
}

func gcpCloudRunUpdateFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_cloud_run_v2_service" "main" {
  name     = "api"
  location = "us-central1"

  labels = {
    env = "staging"
  }

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

func gcpLoadBalancerUpdateFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_compute_global_address" "lb" {
  name = "lb-ip"
}

resource "google_compute_health_check" "hc" {
  name = "lb-hc"

  http_health_check {
    port         = 80
    request_path = "/"
  }
}

resource "google_compute_backend_service" "be" {
  name          = "lb-be"
  description   = "updated-backend"
  protocol      = "HTTP"
  health_checks = [google_compute_health_check.hc.id]
}

resource "google_compute_url_map" "um" {
  name            = "lb-um"
  default_service = google_compute_backend_service.be.id
}

resource "google_compute_ssl_certificate" "cert" {
  name        = "lb-cert"
  private_key = "fake-private-key"
  certificate = "fake-certificate"
}

resource "google_compute_target_https_proxy" "thp" {
  name             = "lb-thp"
  url_map          = google_compute_url_map.um.id
  ssl_certificates = [google_compute_ssl_certificate.cert.id]
}

resource "google_compute_global_forwarding_rule" "fr" {
  name       = "lb-fr"
  ip_address = google_compute_global_address.lb.id
  port_range = "443"
  target     = google_compute_target_https_proxy.thp.id
}
`),
	}
}

func gcpIAMUpdateFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTFV5(fakegcpURL)),
		"main.tf": []byte(`resource "google_service_account" "ci" {
  account_id   = "ci-runner"
  display_name = "CI runner (rotated)"
}

resource "google_service_account_key" "ci" {
  service_account_id = google_service_account.ci.name
  key_algorithm      = "KEY_ALG_RSA_2048"
}
`),
	}
}

func gcpStorageUpdateFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_storage_bucket" "assets" {
  name          = "fake-project-app-assets"
  location      = "us-central1"
  force_destroy = true

  uniform_bucket_level_access = true

  labels = {
    env = "prod"
  }

  encryption {
    default_kms_key_name = "projects/fake-project/locations/us-central1/keyRings/r/cryptoKeys/k"
  }
}
`),
	}
}

func gcpSecretManagerUpdateFiles(fakegcpURL string) map[string][]byte {
	return map[string][]byte{
		"providers.tf": []byte(gcpProviderTF(fakegcpURL)),
		"main.tf": []byte(`resource "google_secret_manager_secret" "main" {
  secret_id = "db-credentials"

  labels = {
    rotation = "monthly"
  }

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

// State extractors used by per-test update verifiers. Each picks a
// single mutable field off the first matching item in the relevant
// fakegcp state collection.

func firstSubscriptionAckDeadline(state map[string]any) int {
	subs := stateSlice(state, "pubsub", "subscriptions")
	if len(subs) == 0 {
		return 0
	}
	v, _ := subs[0]["ackDeadlineSeconds"].(float64)
	return int(v)
}

func firstRecordSetTTL(state map[string]any) int {
	rs := stateSlice(state, "dns", "record_sets")
	if len(rs) == 0 {
		return 0
	}
	v, _ := rs[0]["ttl"].(float64)
	return int(v)
}

func firstCloudRunServiceLabels(state map[string]any) map[string]any {
	svcs := stateSlice(state, "cloudrun", "services")
	if len(svcs) == 0 {
		return nil
	}
	labels, _ := svcs[0]["labels"].(map[string]any)
	return labels
}

func firstBackendServiceDescription(state map[string]any) string {
	bes := stateSlice(state, "lb", "backend_services")
	if len(bes) == 0 {
		return ""
	}
	v, _ := bes[0]["description"].(string)
	return v
}

func firstServiceAccountDisplayName(state map[string]any) string {
	sas := stateSlice(state, "iam", "serviceAccounts")
	if len(sas) == 0 {
		return ""
	}
	v, _ := sas[0]["displayName"].(string)
	return v
}

func firstBucketLabels(state map[string]any) map[string]any {
	buckets := stateSlice(state, "storage", "buckets")
	if len(buckets) == 0 {
		return nil
	}
	labels, _ := buckets[0]["labels"].(map[string]any)
	return labels
}

func firstSecretLabels(state map[string]any) map[string]any {
	secrets := stateSlice(state, "secretmanager", "secrets")
	if len(secrets) == 0 {
		return nil
	}
	labels, _ := secrets[0]["labels"].(map[string]any)
	return labels
}

// collectReplaceLogicalKeys picks the strongest stable identifier
// for items in a replace-allowed collection. The uuid suffix in
// e.g. iam/keys regenerates on every refresh, so we extract a
// parent-resource key (service-account email + key index) instead
// of falling back to count parity. For other replace-allowed
// collections we use the standard logical key (name+type for DNS
// rrsets, name otherwise).
func collectReplaceLogicalKeys(state map[string]any, collectionKey string) []string {
	switch collectionKey {
	case "iam/keys":
		return collectKeyParents(state)
	}
	parts := strings.SplitN(collectionKey, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	keys := collectLogicalKeys(state, []gcpStateExpect{{root: parts[0], collection: parts[1]}})
	return keys[collectionKey]
}

// collectKeyParents returns one entry per IAM key, anchored on its
// parent service-account email plus a stable per-SA index. The
// uuid embedded in the key name regenerates on every refresh, so
// using it would defeat the purpose of the replace-allowed check;
// counting per-SA gives us a key that survives the recreate but
// catches a recreate that rebound the key to a different SA, or a
// recreate that lost or duplicated keys under one SA.
func collectKeyParents(state map[string]any) []string {
	items := stateSlice(state, "iam", "keys")
	perSA := map[string]int{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		email := getKeyParentEmail(item)
		idx := perSA[email]
		perSA[email] = idx + 1
		out = append(out, fmt.Sprintf("sa=%s/i=%d", email, idx))
	}
	return out
}

// getKeyParentEmail extracts the parent service-account email
// from a fakegcp IAM key state item, falling back to the email
// embedded in the key name.
func getKeyParentEmail(item map[string]any) string {
	if email, _ := item["serviceAccountEmail"].(string); email != "" {
		return email
	}
	name, _ := item["name"].(string)
	parts := strings.Split(name, "/")
	if len(parts) >= 4 && parts[len(parts)-2] == "keys" {
		return parts[len(parts)-3]
	}
	return ""
}

// collectLogicalKeys returns, for each (root, collection), a slice
// of "logical" keys identifying each item — name plus type for DNS
// rrsets (since terraform-provider-google addresses them by both),
// plain name elsewhere. Used to assert that replace-allowed
// collections still contain the same set of items pre/post update.
func collectLogicalKeys(state map[string]any, expects []gcpStateExpect) map[string][]string {
	out := map[string][]string{}
	for _, exp := range expects {
		key := exp.root + "/" + exp.collection
		items := stateSlice(state, exp.root, exp.collection)
		keys := make([]string, 0, len(items))
		for _, item := range items {
			name, _ := item["name"].(string)
			rtype, _ := item["type"].(string)
			if rtype != "" {
				keys = append(keys, name+"/"+rtype)
			} else if name != "" {
				keys = append(keys, name)
			}
		}
		out[key] = keys
	}
	return out
}

// collectIdentities returns, for each (root, collection) in expects,
// the set of stable identity keys for items currently in fakegcp state.
// We use `name` because every GCP resource shape we exercise stores
// its fully-qualified resource path there; an in-place update keeps
// it stable, while a delete/recreate produces a different name (or
// at minimum, a different uniqueId for IAM service accounts) — and
// even when the name is reused, sameIdentities also checks uniqueId/
// id when present so recreates surface.
func collectIdentities(state map[string]any, expects []gcpStateExpect) map[string][]string {
	out := map[string][]string{}
	for _, exp := range expects {
		key := exp.root + "/" + exp.collection
		items := stateSlice(state, exp.root, exp.collection)
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, identityKey(item))
		}
		out[key] = ids
	}
	return out
}

// identityKey picks the most stable identifier on a fakegcp state
// item. uniqueId (IAM) and id (DNS zones, compute) take precedence;
// they're server-assigned and change on recreate. Where the resource
// shape doesn't expose a server id, we fall back to the pair
// (name, creation-timestamp). The timestamp is critical: a delete +
// recreate under the same logical name produces a fresh
// createTime/creationTimestamp, so the identity flips even though
// `name` is reused. Without it we'd silently miss same-name
// recreates for resources like DNS record sets.
func identityKey(item map[string]any) string {
	for _, field := range []string{"uniqueId", "id"} {
		switch v := item[field].(type) {
		case string:
			if v != "" {
				return field + "=" + v
			}
		case float64:
			return fmt.Sprintf("%s=%v", field, v)
		}
	}
	name, _ := item["name"].(string)
	ts := firstStringField(item, "createTime", "creationTimestamp", "creationTime")
	switch {
	case name != "" && ts != "":
		return "name=" + name + "@" + ts
	case name != "":
		return "name=" + name
	case ts != "":
		return "createTime=" + ts
	}
	return ""
}

func firstStringField(item map[string]any, fields ...string) string {
	for _, f := range fields {
		if v, _ := item[f].(string); v != "" {
			return v
		}
	}
	return ""
}

func sameIdentities(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, k := range a {
		seen[k]++
	}
	for _, k := range b {
		if seen[k] == 0 {
			return false
		}
		seen[k]--
	}
	return true
}

// stateSlice returns the typed object slice at state[root][collection]
// or nil. fakegcp items are unmarshaled into map[string]any when JSON
// is decoded into map[string]any.
func stateSlice(state map[string]any, root, collection string) []map[string]any {
	rootMap, ok := state[root].(map[string]any)
	if !ok {
		return nil
	}
	items, ok := rootMap[collection].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
