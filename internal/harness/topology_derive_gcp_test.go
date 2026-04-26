package harness

import (
	"encoding/json"
	"testing"
)

// TestDeriveTopologyDispatchesGCPByDetection confirms that DeriveTopology
// recognizes a fakegcp-shaped state (top-level `compute` key) and routes
// it through the GCP path rather than the Scaleway path. The Scaleway
// rawMockState struct has no `compute` field and would silently produce
// an empty topology if it accidentally consumed a GCP payload — this
// test is the regression boundary.
func TestDeriveTopologyDispatchesGCPByDetection(t *testing.T) {
	t.Parallel()

	if got := detectCloud([]byte(`{"compute":{"instances":[]}}`)); got != "gcp" {
		t.Fatalf("expected gcp detection, got %q", got)
	}
	if got := detectCloud([]byte(`{"instance":{"servers":[]}}`)); got != "scaleway" {
		t.Fatalf("expected scaleway detection, got %q", got)
	}
	if got := detectCloud([]byte(`{}`)); got != "scaleway" {
		t.Fatalf("expected scaleway default for empty state, got %q", got)
	}
	if got := detectCloud([]byte(`not json`)); got != "scaleway" {
		t.Fatalf("expected scaleway default for malformed json, got %q", got)
	}
}

// TestDeriveTopologyGCPHTTPProbeWithBackend covers the canonical happy
// path: a global forwarding rule on port 80 plus a backend service with
// at least one backend → http_probe load_balancer:80 = true.
func TestDeriveTopologyGCPHTTPProbeWithBackend(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{
			"instances": []map[string]any{{"name": "web-0"}},
		},
		"lb": map[string]any{
			"global_forwarding_rules": []map[string]any{
				{"port_range": "80", "target": "https://example/proxy"},
			},
			"backend_services": []map[string]any{
				{"backends": []any{map[string]any{"group": "ig"}}},
			},
		},
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	out, diagnostics, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	var parsed struct {
		HTTPProbe map[string]bool `json:"http_probe"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got, ok := parsed.HTTPProbe["load_balancer:80"]; !ok || !got {
		t.Fatalf("expected load_balancer:80=true, got %+v", parsed.HTTPProbe)
	}
	// Diagnostics may include the LB-level fallback for off-port probes;
	// the per-port entry should not have a diagnostic since it's true.
	if msg := diagnostics["load_balancer:80"]; msg != "" {
		t.Fatalf("expected no diagnostic for healthy probe, got %q", msg)
	}
}

// TestDeriveTopologyGCPHTTPProbeWithoutBackend covers the negative path
// where a forwarding rule exists but no backend service has backends.
// Both the per-port entry and the LB-level fallback should carry the
// "no backend" diagnostic.
func TestDeriveTopologyGCPHTTPProbeWithoutBackend(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{},
		"lb": map[string]any{
			"global_forwarding_rules": []map[string]any{
				{"port_range": "443"},
			},
			"backend_services": []map[string]any{
				{"backends": []any{}},
			},
		},
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	out, diagnostics, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	var parsed struct {
		HTTPProbe map[string]bool `json:"http_probe"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.HTTPProbe["load_balancer:443"] {
		t.Fatalf("expected load_balancer:443=false, got true")
	}
	if msg := diagnostics["load_balancer:443"]; msg == "" {
		t.Fatalf("expected per-port diagnostic, got empty")
	}
	if msg := diagnostics["load_balancer"]; msg == "" {
		t.Fatalf("expected LB-level diagnostic fallback, got empty")
	}
}

// TestDeriveTopologyGCPConnectivityComputeToDatabase confirms that a
// scenario containing both compute instances and a Cloud SQL instance
// yields compute → database edges on the standard ports.
func TestDeriveTopologyGCPConnectivityComputeToDatabase(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{
			"instances": []map[string]any{{"name": "web-0"}},
		},
		"sql": map[string]any{
			"instances": []map[string]any{{"name": "main-db"}},
		},
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	var parsed struct {
		Connectivity map[string]bool `json:"connectivity"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !parsed.Connectivity["compute->database:5432"] {
		t.Fatalf("expected compute->database:5432=true")
	}
	if parsed.Connectivity["public_internet->database:5432"] {
		t.Fatalf("expected public_internet->database:5432=false (Cloud SQL ACL)")
	}
}

// TestDeriveTopologyGCPPublicIngressFromFirewall checks that a 0.0.0.0/0
// allow-INGRESS firewall on a project flips public_internet→compute
// connectivity to true. Without such a rule, default-deny applies.
func TestDeriveTopologyGCPPublicIngressFromFirewall(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{
			"instances": []map[string]any{{"name": "web-0"}},
			"firewalls": []map[string]any{
				{
					"direction":     "INGRESS",
					"source_ranges": []any{"0.0.0.0/0"},
					"allowed":       []any{map[string]any{"IPProtocol": "tcp"}},
				},
			},
		},
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	var parsed struct {
		Connectivity map[string]bool `json:"connectivity"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !parsed.Connectivity["public_internet->compute"] {
		t.Fatalf("expected public_internet->compute=true with 0.0.0.0/0 allow rule")
	}
}

// TestDeriveTopologyGCPNoFirewallDefaultDeny ensures the default with no
// firewall rules is private-only.
func TestDeriveTopologyGCPNoFirewallDefaultDeny(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{
			"instances": []map[string]any{{"name": "web-0"}},
		},
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	var parsed struct {
		Connectivity map[string]bool `json:"connectivity"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Connectivity["public_internet->compute"] {
		t.Fatalf("expected public_internet->compute=false without 0.0.0.0/0 firewall")
	}
}
