package harness

import (
	"encoding/json"
	"testing"
)

// A public load balancer accepts TCP on its frontend port even with no
// backend servers attached — real Scaleway completes the handshake and
// then answers HTTP 503. The mock must agree, or a scenario that declares
// no compute (lb-paris) passes `connectivity` against real infrastructure
// and fails it against the mock.
func TestDeriveTopologyEmitsLoadBalancerConnectivityWithoutBackend(t *testing.T) {
	state := []byte(`{
      "lb": {
        "lbs":       [{"id": "lb-1"}],
        "ips":       [{"id": "ip-1", "lb_id": "lb-1"}],
        "frontends": [{"id": "fe-1", "lb_id": "lb-1", "inbound_port": 80}],
        "backends":  []
      }
    }`)

	derived, _, err := DeriveTopology(state)
	if err != nil {
		t.Fatalf("derive topology: %v", err)
	}
	var out struct {
		Connectivity map[string]bool `json:"connectivity"`
		HTTPProbe    map[string]bool `json:"http_probe"`
	}
	if err := json.Unmarshal(derived, &out); err != nil {
		t.Fatalf("decode derived topology: %v", err)
	}

	connKey := connectivityKey("public_internet", "load_balancer", 80)
	if !out.Connectivity[connKey] {
		t.Errorf("expected %s to be reachable at TCP level; got %v", connKey, out.Connectivity)
	}
	// The HTTP claim is strictly stronger and must stay false without a
	// backend, or the two probe types would be indistinguishable.
	if out.HTTPProbe[httpProbeKey("load_balancer", 80)] {
		t.Errorf("http_probe must not report reachable with no backend: %v", out.HTTPProbe)
	}
}

// No IP means nothing to connect to.
func TestDeriveTopologyOmitsLoadBalancerConnectivityWithoutIP(t *testing.T) {
	state := []byte(`{
      "lb": {
        "lbs":       [{"id": "lb-1"}],
        "ips":       [],
        "frontends": [{"id": "fe-1", "lb_id": "lb-1", "inbound_port": 80}]
      }
    }`)

	derived, _, err := DeriveTopology(state)
	if err != nil {
		t.Fatalf("derive topology: %v", err)
	}
	var out struct {
		Connectivity map[string]bool `json:"connectivity"`
	}
	if err := json.Unmarshal(derived, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Connectivity[connectivityKey("public_internet", "load_balancer", 80)] {
		t.Errorf("an LB with no IP must not be reported reachable: %v", out.Connectivity)
	}
}
