package harness

import (
	"encoding/json"
	"testing"
)

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func deriveAndParse(t *testing.T, state map[string]any) (map[string]bool, map[string]bool) {
	t.Helper()
	conn, probe, _ := deriveAndParseAll(t, state)
	return conn, probe
}

func deriveAndParseAll(t *testing.T, state map[string]any) (map[string]bool, map[string]bool, map[string]string) {
	t.Helper()
	raw := mustMarshal(t, state)
	out, diagnostics, err := DeriveTopology(raw)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Connectivity map[string]bool `json:"connectivity"`
		HTTPProbe    map[string]bool `json:"http_probe"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	return result.Connectivity, result.HTTPProbe, diagnostics
}

func webAppState() map[string]any {
	pnID := "pn-aaa-bbb"
	lbID := "lb-111"
	serverID := "srv-222"
	return map[string]any{
		"instance": map[string]any{
			"servers":      []any{map[string]any{"id": serverID}},
			"ips":          []any{map[string]any{"id": "ip-1", "server": map[string]any{"id": serverID}}},
			"private_nics": []any{map[string]any{"server_id": serverID, "private_network_id": pnID}},
		},
		"lb": map[string]any{
			"lbs":       []any{map[string]any{"id": lbID}},
			"ips":       []any{map[string]any{"id": "lbip-1", "lb_id": lbID}},
			"frontends": []any{map[string]any{"id": "fe-1", "lb_id": lbID, "inbound_port": float64(80)}},
			"backends":  []any{map[string]any{"id": "be-1", "lb_id": lbID}},
		},
		"rdb": map[string]any{
			"instances": []any{map[string]any{
				"id": "rdb-1",
				"endpoints": []any{map[string]any{
					"port":            float64(5432),
					"private_network": map[string]any{"id": pnID},
				}},
			}},
		},
	}
}

func TestDeriveTopologyWebApp(t *testing.T) {
	conn, probe := deriveAndParse(t, webAppState())

	if !probe["load_balancer:80"] {
		t.Error("expected http_probe[load_balancer:80] = true")
	}
	if !conn["compute->database:5432"] {
		t.Error("expected connectivity[compute->database:5432] = true")
	}
	if conn["public_internet->database:5432"] {
		t.Error("expected connectivity[public_internet->database:5432] = false")
	}
	if !conn["public_internet->compute"] {
		t.Error("expected connectivity[public_internet->compute] = true (server has public IP)")
	}
}

func TestDeriveTopologyNoLB(t *testing.T) {
	state := map[string]any{
		"instance": map[string]any{
			"servers": []any{map[string]any{"id": "srv-1"}},
		},
	}
	_, probe := deriveAndParse(t, state)
	if len(probe) != 0 {
		t.Errorf("expected empty http_probe, got %v", probe)
	}
}

func TestDeriveTopologyLBNoBackend(t *testing.T) {
	lbID := "lb-1"
	state := map[string]any{
		"lb": map[string]any{
			"lbs":       []any{map[string]any{"id": lbID}},
			"ips":       []any{map[string]any{"id": "lbip-1", "lb_id": lbID}},
			"frontends": []any{map[string]any{"id": "fe-1", "lb_id": lbID, "inbound_port": float64(80)}},
			"backends":  []any{},
		},
	}
	_, probe := deriveAndParse(t, state)
	if probe["load_balancer:80"] {
		t.Error("expected http_probe[load_balancer:80] = false when no backend")
	}
	// Key should exist but be false.
	if _, ok := probe["load_balancer:80"]; !ok {
		t.Error("expected key load_balancer:80 to exist in http_probe")
	}
}

func TestDeriveTopologyLBNoIP(t *testing.T) {
	lbID := "lb-1"
	state := map[string]any{
		"lb": map[string]any{
			"lbs":       []any{map[string]any{"id": lbID}},
			"ips":       []any{},
			"frontends": []any{map[string]any{"id": "fe-1", "lb_id": lbID, "inbound_port": float64(80)}},
			"backends":  []any{map[string]any{"id": "be-1", "lb_id": lbID}},
		},
	}
	_, probe := deriveAndParse(t, state)
	if probe["load_balancer:80"] {
		t.Error("expected http_probe[load_balancer:80] = false when no IP")
	}
}

func TestDeriveTopologyPublicDB(t *testing.T) {
	state := map[string]any{
		"instance": map[string]any{
			"servers": []any{map[string]any{"id": "srv-1"}},
		},
		"rdb": map[string]any{
			"instances": []any{map[string]any{
				"id": "rdb-1",
				"endpoints": []any{map[string]any{
					"port": float64(5432),
				}},
			}},
		},
	}
	conn, _ := deriveAndParse(t, state)
	if !conn["public_internet->database:5432"] {
		t.Error("expected connectivity[public_internet->database:5432] = true for public endpoint")
	}
}

func TestDeriveTopologyEmptyState(t *testing.T) {
	conn, probe := deriveAndParse(t, map[string]any{})
	if conn == nil {
		t.Error("expected non-nil connectivity map")
	}
	if probe == nil {
		t.Error("expected non-nil http_probe map")
	}
	if len(conn) != 0 {
		t.Errorf("expected empty connectivity, got %v", conn)
	}
	if len(probe) != 0 {
		t.Errorf("expected empty http_probe, got %v", probe)
	}
}

func TestDeriveTopologyMySQLPort(t *testing.T) {
	pnID := "pn-mysql"
	state := map[string]any{
		"instance": map[string]any{
			"servers":      []any{map[string]any{"id": "srv-1"}},
			"private_nics": []any{map[string]any{"server_id": "srv-1", "private_network_id": pnID}},
		},
		"rdb": map[string]any{
			"instances": []any{map[string]any{
				"id": "rdb-1",
				"endpoints": []any{map[string]any{
					"port":            float64(3306),
					"private_network": map[string]any{"id": pnID},
				}},
			}},
		},
	}
	conn, _ := deriveAndParse(t, state)
	if !conn["compute->database:3306"] {
		t.Error("expected connectivity[compute->database:3306] = true")
	}
}

func TestDeriveTopologyRedis(t *testing.T) {
	pnID := "pn-redis"
	state := map[string]any{
		"instance": map[string]any{
			"servers":      []any{map[string]any{"id": "srv-1"}},
			"private_nics": []any{map[string]any{"server_id": "srv-1", "private_network_id": pnID}},
		},
		"redis": map[string]any{
			"clusters": []any{map[string]any{
				"id": "redis-1",
				"endpoints": []any{map[string]any{
					"port":            float64(6379),
					"private_network": map[string]any{"id": pnID},
				}},
			}},
		},
	}
	conn, _ := deriveAndParse(t, state)
	if !conn["compute->redis:6379"] {
		t.Error("expected connectivity[compute->redis:6379] = true")
	}
}

func TestEvaluateTopologyWithRawState(t *testing.T) {
	state := webAppState()
	raw := mustMarshal(t, state)

	checks := []TopologyCheck{
		{Type: "http_probe", Target: "load_balancer", Port: 80, Expect: "reachable"},
		{Type: "connectivity", From: "compute", To: "database", Port: 5432, Expect: "success"},
		{Type: "connectivity", From: "public_internet", To: "database", Port: 5432, Expect: "failure"},
	}

	failures, err := EvaluateTopology(raw, checks)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 {
		for _, f := range failures {
			t.Errorf("unexpected failure: %s", f.Detail)
		}
	}
}

func TestDeriveTopologyRDBNonMatchingPN(t *testing.T) {
	state := map[string]any{
		"instance": map[string]any{
			"servers":      []any{map[string]any{"id": "srv-1"}},
			"private_nics": []any{map[string]any{"server_id": "srv-1", "private_network_id": "pn-aaa"}},
		},
		"rdb": map[string]any{
			"instances": []any{map[string]any{
				"id": "rdb-1",
				"endpoints": []any{map[string]any{
					"port":            float64(5432),
					"private_network": map[string]any{"id": "pn-zzz"},
				}},
			}},
		},
	}
	conn, _ := deriveAndParse(t, state)
	if conn["compute->database:5432"] {
		t.Error("expected no connectivity when server and RDB are on different private networks")
	}
}

func TestDeriveTopologyDiagnosticsNoBackend(t *testing.T) {
	lbID := "lb-1"
	state := map[string]any{
		"lb": map[string]any{
			"lbs":       []any{map[string]any{"id": lbID}},
			"ips":       []any{map[string]any{"id": "lbip-1", "lb_id": lbID}},
			"frontends": []any{map[string]any{"id": "fe-1", "lb_id": lbID, "inbound_port": float64(80)}},
			"backends":  []any{},
		},
	}
	_, probe, diagnostics := deriveAndParseAll(t, state)
	if probe["load_balancer:80"] {
		t.Fatal("expected http_probe[load_balancer:80] = false")
	}
	got, ok := diagnostics["load_balancer:80"]
	if !ok {
		t.Fatalf("expected diagnostics[load_balancer:80] to be set, got %v", diagnostics)
	}
	if got != "no backend attached" {
		t.Errorf("expected 'no backend attached', got %q", got)
	}
}

func TestDeriveTopologyDiagnosticsNoIP(t *testing.T) {
	lbID := "lb-1"
	state := map[string]any{
		"lb": map[string]any{
			"lbs":       []any{map[string]any{"id": lbID}},
			"ips":       []any{},
			"frontends": []any{map[string]any{"id": "fe-1", "lb_id": lbID, "inbound_port": float64(80)}},
			"backends":  []any{map[string]any{"id": "be-1", "lb_id": lbID}},
		},
	}
	_, probe, diagnostics := deriveAndParseAll(t, state)
	if probe["load_balancer:80"] {
		t.Fatal("expected http_probe[load_balancer:80] = false")
	}
	got, ok := diagnostics["load_balancer:80"]
	if !ok {
		t.Fatalf("expected diagnostics[load_balancer:80] to be set, got %v", diagnostics)
	}
	if got != "no public ip on lb" {
		t.Errorf("expected 'no public ip on lb', got %q", got)
	}
}

func TestDeriveTopologyDiagnosticsNoBackendNoIP(t *testing.T) {
	lbID := "lb-1"
	state := map[string]any{
		"lb": map[string]any{
			"lbs":       []any{map[string]any{"id": lbID}},
			"ips":       []any{},
			"frontends": []any{map[string]any{"id": "fe-1", "lb_id": lbID, "inbound_port": float64(80)}},
			"backends":  []any{},
		},
	}
	_, _, diagnostics := deriveAndParseAll(t, state)
	got, ok := diagnostics["load_balancer:80"]
	if !ok {
		t.Fatalf("expected diagnostics[load_balancer:80] to be set, got %v", diagnostics)
	}
	if got != "no backend attached and no public ip on lb" {
		t.Errorf("expected combined reason, got %q", got)
	}
}

func TestDeriveTopologyDiagnosticsNoFrontendOnRequestedPort(t *testing.T) {
	// LB has no frontends at all. A probe targeting port 80 has no
	// http_probe key, so the diagnostic is keyed by the LB-level fallback.
	lbID := "lb-1"
	state := map[string]any{
		"lb": map[string]any{
			"lbs":       []any{map[string]any{"id": lbID}},
			"ips":       []any{map[string]any{"id": "lbip-1", "lb_id": lbID}},
			"frontends": []any{},
			"backends":  []any{map[string]any{"id": "be-1", "lb_id": lbID}},
		},
	}
	_, probe, diagnostics := deriveAndParseAll(t, state)
	if _, ok := probe["load_balancer:80"]; ok {
		t.Fatalf("did not expect http_probe[load_balancer:80] entry, got %v", probe)
	}
	got, ok := diagnostics["load_balancer"]
	if !ok {
		t.Fatalf("expected diagnostics[load_balancer] fallback, got %v", diagnostics)
	}
	if got != "no frontends configured on lb" {
		t.Errorf("expected 'no frontends configured on lb', got %q", got)
	}
}

func TestDeriveTopologyDiagnosticsFrontendOnDifferentPort(t *testing.T) {
	// Frontend exists on 443; a probe for 80 has no http_probe key. The
	// LB-level fallback diagnostic lists the actual frontend ports.
	lbID := "lb-1"
	state := map[string]any{
		"lb": map[string]any{
			"lbs":       []any{map[string]any{"id": lbID}},
			"ips":       []any{map[string]any{"id": "lbip-1", "lb_id": lbID}},
			"frontends": []any{map[string]any{"id": "fe-1", "lb_id": lbID, "inbound_port": float64(443)}},
			"backends":  []any{map[string]any{"id": "be-1", "lb_id": lbID}},
		},
	}
	_, probe, diagnostics := deriveAndParseAll(t, state)
	if _, ok := probe["load_balancer:80"]; ok {
		t.Fatalf("did not expect http_probe[load_balancer:80] entry, got %v", probe)
	}
	if !probe["load_balancer:443"] {
		t.Errorf("expected http_probe[load_balancer:443] = true, got %v", probe)
	}
	got, ok := diagnostics["load_balancer"]
	if !ok {
		t.Fatalf("expected diagnostics[load_balancer] fallback, got %v", diagnostics)
	}
	if got != "frontends on port 443" {
		t.Errorf("expected 'frontends on port 443', got %q", got)
	}
}

func TestDeriveTopologyDiagnosticsHealthyProbeHasNoPortEntry(t *testing.T) {
	// A fully wired LB has no port-specific diagnostic for a reachable
	// probe; the only entry is the LB-level fallback (kept so probes on
	// other ports can still surface what frontends do exist).
	state := webAppState()
	_, probe, diagnostics := deriveAndParseAll(t, state)
	if !probe["load_balancer:80"] {
		t.Fatal("expected probe to be reachable")
	}
	if _, ok := diagnostics["load_balancer:80"]; ok {
		t.Errorf("did not expect diagnostic for healthy probe, got %v", diagnostics)
	}
	if got := diagnostics["load_balancer"]; got != "frontends on port 80" {
		t.Errorf("expected fallback 'frontends on port 80', got %q", got)
	}
}

func TestDeriveTopologyRedisNoPrivateNetwork(t *testing.T) {
	state := map[string]any{
		"instance": map[string]any{
			"servers":      []any{map[string]any{"id": "srv-1"}},
			"private_nics": []any{map[string]any{"server_id": "srv-1", "private_network_id": "pn-aaa"}},
		},
		"redis": map[string]any{
			"clusters": []any{map[string]any{
				"id": "redis-1",
				"endpoints": []any{map[string]any{
					"port": float64(6379),
				}},
			}},
		},
	}
	conn, _ := deriveAndParse(t, state)
	if conn["compute->redis:6379"] {
		t.Error("expected no connectivity when redis has no private_network in endpoint")
	}
}
