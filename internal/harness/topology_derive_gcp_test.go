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

// TestDeriveTopologyGCPMultipleForwardingRules covers an L4/L7 LB pair on
// distinct ports. Both should appear in http_probe with the same backend
// status.
func TestDeriveTopologyGCPMultipleForwardingRules(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{},
		"lb": map[string]any{
			"global_forwarding_rules": []map[string]any{
				{"port_range": "80"},
				{"port_range": "443"},
			},
			"backend_services": []map[string]any{
				{"backends": []any{map[string]any{"group": "ig"}}},
			},
		},
	}
	stateJSON, _ := json.Marshal(state)

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var parsed struct {
		HTTPProbe map[string]bool `json:"http_probe"`
	}
	_ = json.Unmarshal(out, &parsed)
	for _, key := range []string{"load_balancer:80", "load_balancer:443"} {
		if !parsed.HTTPProbe[key] {
			t.Fatalf("expected %s=true, got %+v", key, parsed.HTTPProbe)
		}
	}
}

// TestDeriveTopologyGCPForwardingRulePortRange handles "80-80" port range
// and a `ports` array variant. Both should yield port=80 entries.
func TestDeriveTopologyGCPForwardingRulePortRange(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{},
		"lb": map[string]any{
			"global_forwarding_rules": []map[string]any{
				{"port_range": "80-80"},
				{"ports": []any{"443"}},
			},
			"backend_services": []map[string]any{
				{"backends": []any{map[string]any{"group": "ig"}}},
			},
		},
	}
	stateJSON, _ := json.Marshal(state)

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var parsed struct {
		HTTPProbe map[string]bool `json:"http_probe"`
	}
	_ = json.Unmarshal(out, &parsed)
	for _, key := range []string{"load_balancer:80", "load_balancer:443"} {
		if _, ok := parsed.HTTPProbe[key]; !ok {
			t.Fatalf("expected %s entry, got %+v", key, parsed.HTTPProbe)
		}
	}
}

// TestDeriveTopologyGCPDatabaseOnlyHasNoComputeEdge confirms that a
// scenario with just a Cloud SQL instance produces no compute->database
// edge (since there is no compute).
func TestDeriveTopologyGCPDatabaseOnlyHasNoComputeEdge(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{},
		"sql": map[string]any{
			"instances": []map[string]any{{"name": "main-db"}},
		},
	}
	stateJSON, _ := json.Marshal(state)

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var parsed struct {
		Connectivity map[string]bool `json:"connectivity"`
	}
	_ = json.Unmarshal(out, &parsed)
	if _, ok := parsed.Connectivity["compute->database:5432"]; ok {
		t.Fatalf("expected no compute->database edge without compute, got %+v", parsed.Connectivity)
	}
}

// TestDeriveTopologyGCPMySQLPort covers engine-aware port selection:
// a MYSQL_8_0 instance produces a 3306 edge, not 5432. Mixing two
// instances (postgres + mysql) should produce both edges.
func TestDeriveTopologyGCPMySQLPort(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{
			"instances": []map[string]any{{"name": "web-0"}},
		},
		"sql": map[string]any{
			"instances": []map[string]any{
				{"name": "pg-db", "databaseVersion": "POSTGRES_15"},
				{"name": "my-db", "databaseVersion": "MYSQL_8_0"},
			},
		},
	}
	stateJSON, _ := json.Marshal(state)

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var parsed struct {
		Connectivity map[string]bool `json:"connectivity"`
	}
	_ = json.Unmarshal(out, &parsed)
	if !parsed.Connectivity["compute->database:5432"] {
		t.Fatalf("expected compute->database:5432=true (postgres), got %+v", parsed.Connectivity)
	}
	if !parsed.Connectivity["compute->database:3306"] {
		t.Fatalf("expected compute->database:3306=true (mysql), got %+v", parsed.Connectivity)
	}
}

// TestDeriveTopologyGCPSQLPostgresOnlyOmitsMySQLPort guards against the
// pre-pass-2 behaviour where every SQL instance produced both 5432 and
// 3306 edges regardless of engine — a postgres-only stack should NOT
// surface 3306.
func TestDeriveTopologyGCPSQLPostgresOnlyOmitsMySQLPort(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{
			"instances": []map[string]any{{"name": "web-0"}},
		},
		"sql": map[string]any{
			"instances": []map[string]any{{"name": "pg-db", "databaseVersion": "POSTGRES_15"}},
		},
	}
	stateJSON, _ := json.Marshal(state)

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var parsed struct {
		Connectivity map[string]bool `json:"connectivity"`
	}
	_ = json.Unmarshal(out, &parsed)
	if !parsed.Connectivity["compute->database:5432"] {
		t.Fatalf("expected compute->database:5432=true, got %+v", parsed.Connectivity)
	}
	if _, ok := parsed.Connectivity["compute->database:3306"]; ok {
		t.Fatalf("expected no compute->database:3306 edge for postgres-only, got %+v", parsed.Connectivity)
	}
}

// TestDeriveTopologyGCPSQLPublicReachableViaAuthorizedNetworks pins the
// new public-reachability rule: a 0.0.0.0/0 authorized network flips
// public_internet->database:5432 to true regardless of ipv4Enabled.
func TestDeriveTopologyGCPSQLPublicReachableViaAuthorizedNetworks(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{
			"instances": []map[string]any{{"name": "web-0"}},
		},
		"sql": map[string]any{
			"instances": []map[string]any{{
				"name":            "exposed-db",
				"databaseVersion": "POSTGRES_15",
				"settings": map[string]any{
					"ipConfiguration": map[string]any{
						"ipv4Enabled": true,
						"authorizedNetworks": []any{
							map[string]any{"value": "0.0.0.0/0"},
						},
					},
				},
			}},
		},
	}
	stateJSON, _ := json.Marshal(state)

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var parsed struct {
		Connectivity map[string]bool `json:"connectivity"`
	}
	_ = json.Unmarshal(out, &parsed)
	if !parsed.Connectivity["public_internet->database:5432"] {
		t.Fatalf("expected public_internet->database:5432=true with 0.0.0.0/0 authorized network, got %+v", parsed.Connectivity)
	}
}

// TestDeriveTopologyGCPKubernetesEdge confirms compute->kubernetes
// connectivity surfaces when a GKE cluster is in state.
func TestDeriveTopologyGCPKubernetesEdge(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{
			"instances": []map[string]any{{"name": "web-0"}},
		},
		"container": map[string]any{
			"clusters": []map[string]any{{"name": "gke-main"}},
		},
	}
	stateJSON, _ := json.Marshal(state)

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var parsed struct {
		Connectivity map[string]bool `json:"connectivity"`
	}
	_ = json.Unmarshal(out, &parsed)
	if !parsed.Connectivity["compute->kubernetes"] {
		t.Fatalf("expected compute->kubernetes=true, got %+v", parsed.Connectivity)
	}
}

// TestDeriveTopologyGCPMalformedNestedShape exercises the GCP-path
// unmarshal error: detectCloud succeeds (top-level `compute` parses
// cleanly), but the nested `instances` field's wrong type fails
// rawGCPState's strict decode. The previous test sent malformed
// top-level JSON which actually fell back to the Scaleway path —
// this version pins the GCP-specific error.
func TestDeriveTopologyGCPMalformedNestedShape(t *testing.T) {
	t.Parallel()

	// `instances` should be an array; passing a string forces
	// json.Unmarshal into rawGCPState to fail on the GCP path.
	payload := []byte(`{"compute":{"instances":"not-an-array"}}`)
	if _, _, err := DeriveTopology(payload); err == nil {
		t.Fatal("expected error for malformed gcp state")
	}
}

// TestDeriveTopologyGCPSQLNarrowAuthorizedNetworkIsPrivate guards the
// pass-3 fix where a single-bastion-IP authorized_networks entry was
// incorrectly flipping public_internet->database to true.
func TestDeriveTopologyGCPSQLNarrowAuthorizedNetworkIsPrivate(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{
			"instances": []map[string]any{{"name": "web-0"}},
		},
		"sql": map[string]any{
			"instances": []map[string]any{{
				"name":            "narrow-db",
				"databaseVersion": "POSTGRES_15",
				"settings": map[string]any{
					"ipConfiguration": map[string]any{
						"ipv4Enabled": true,
						"authorizedNetworks": []any{
							map[string]any{"value": "203.0.113.5/32"},
						},
					},
				},
			}},
		},
	}
	stateJSON, _ := json.Marshal(state)

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var parsed struct {
		Connectivity map[string]bool `json:"connectivity"`
	}
	_ = json.Unmarshal(out, &parsed)
	if parsed.Connectivity["public_internet->database:5432"] {
		t.Fatalf("expected public_internet->database:5432=false with narrow allowlist, got true")
	}
}

// TestDeriveTopologyGCPForwardingRuleOutOfRangePort guards the pass-4
// fix where a port_range like "70000" used to produce a bogus
// load_balancer:70000 http_probe entry instead of being skipped.
func TestDeriveTopologyGCPForwardingRuleOutOfRangePort(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{},
		"lb": map[string]any{
			"global_forwarding_rules": []map[string]any{
				{"port_range": "70000"},
				{"port_range": "0"},
			},
			"backend_services": []map[string]any{
				{"backends": []any{map[string]any{"group": "ig"}}},
			},
		},
	}
	stateJSON, _ := json.Marshal(state)

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var parsed struct {
		HTTPProbe map[string]bool `json:"http_probe"`
	}
	_ = json.Unmarshal(out, &parsed)
	for key := range parsed.HTTPProbe {
		t.Fatalf("expected no http_probe entries for out-of-range ports, got %s", key)
	}
}

// TestDeriveTopologyGCPSQLUnknownEngineProducesNoEdge guards the
// pass-3 fix where unknown engines (e.g. ORACLE_*) used to silently
// produce a postgres-port edge.
func TestDeriveTopologyGCPSQLUnknownEngineProducesNoEdge(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{
			"instances": []map[string]any{{"name": "web-0"}},
		},
		"sql": map[string]any{
			"instances": []map[string]any{{"name": "oracle-db", "databaseVersion": "ORACLE_18"}},
		},
	}
	stateJSON, _ := json.Marshal(state)

	out, _, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	var parsed struct {
		Connectivity map[string]bool `json:"connectivity"`
	}
	_ = json.Unmarshal(out, &parsed)
	for _, key := range []string{"compute->database:5432", "compute->database:3306", "compute->database:1433"} {
		if _, ok := parsed.Connectivity[key]; ok {
			t.Fatalf("expected no compute->database edge for unknown engine, got %s in %+v", key, parsed.Connectivity)
		}
	}
}

// TestDeriveTopologyGCPForwardingRuleUnparseablePort guards the
// distinction between "no rules at all" and "rules with no parseable
// port". Both are misconfigurations but they need different
// diagnostics — silent fall-through to "no forwarding rules
// configured" hides the real problem (fakegcp emitting a port format
// the derivation doesn't recognise).
func TestDeriveTopologyGCPForwardingRuleUnparseablePort(t *testing.T) {
	t.Parallel()

	state := map[string]any{
		"compute": map[string]any{},
		"lb": map[string]any{
			"global_forwarding_rules": []map[string]any{
				{"port_range": ""},
				{"name": "no-port-info"},
			},
			"backend_services": []map[string]any{
				{"backends": []any{map[string]any{"group": "ig"}}},
			},
		},
	}
	stateJSON, _ := json.Marshal(state)

	_, diagnostics, err := DeriveTopology(stateJSON)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if got := diagnostics["load_balancer"]; got != "forwarding rules with no parseable port" {
		t.Fatalf("expected load_balancer diagnostic to flag unparseable port, got %q", got)
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
