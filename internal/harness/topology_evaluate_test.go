package harness

import (
	"strings"
	"testing"
)

// lbStateNoBackend builds a raw mock state with one LB that has an IP and a
// frontend on port 80 but no backends — http_probe[load_balancer:80] = false.
func lbStateNoBackend() map[string]any {
	lbID := "lb-1"
	return map[string]any{
		"lb": map[string]any{
			"lbs":       []any{map[string]any{"id": lbID}},
			"ips":       []any{map[string]any{"id": "lbip-1", "lb_id": lbID}},
			"frontends": []any{map[string]any{"id": "fe-1", "lb_id": lbID, "inbound_port": float64(80)}},
			"backends":  []any{},
		},
	}
}

// lbStateFrontendOnDifferentPort builds a raw mock state where the frontend
// listens on 443 only, so a probe targeting 80 has no http_probe entry and
// must use the LB-level fallback diagnostic.
func lbStateFrontendOnDifferentPort() map[string]any {
	lbID := "lb-1"
	return map[string]any{
		"lb": map[string]any{
			"lbs":       []any{map[string]any{"id": lbID}},
			"ips":       []any{map[string]any{"id": "lbip-1", "lb_id": lbID}},
			"frontends": []any{map[string]any{"id": "fe-1", "lb_id": lbID, "inbound_port": float64(443)}},
			"backends":  []any{map[string]any{"id": "be-1", "lb_id": lbID}},
		},
	}
}

func TestEvaluateTopology_HTTPProbeFailureExactDiagnostic(t *testing.T) {
	t.Parallel()
	raw := mustMarshal(t, lbStateNoBackend())
	checks := []TopologyCheck{
		{Type: "http_probe", Target: "load_balancer", Port: 80, Expect: "reachable"},
	}

	failures, err := EvaluateTopology(raw, checks)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d (%+v)", len(failures), failures)
	}
	got := failures[0].Detail
	if !strings.Contains(got, `"load_balancer:80"`) {
		t.Errorf("expected detail to mention probe key, got %q", got)
	}
	if !strings.Contains(got, "no backend attached") {
		t.Errorf("expected detail to surface exact-key diagnostic, got %q", got)
	}
}

func TestEvaluateTopology_HTTPProbeFailureFallbackDiagnostic(t *testing.T) {
	t.Parallel()
	raw := mustMarshal(t, lbStateFrontendOnDifferentPort())
	checks := []TopologyCheck{
		// Probe targets port 80; the only frontend is on 443, so there's no
		// exact "load_balancer:80" key. The "load_balancer" fallback should
		// surface instead.
		{Type: "http_probe", Target: "load_balancer", Port: 80, Expect: "reachable"},
	}

	failures, err := EvaluateTopology(raw, checks)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d (%+v)", len(failures), failures)
	}
	got := failures[0].Detail
	if !strings.Contains(got, "frontends on port 443") {
		t.Errorf("expected detail to surface LB-level fallback diagnostic, got %q", got)
	}
}

func TestEvaluateTopology_HTTPProbeFailureNoDiagnosticKeepsBareMessage(t *testing.T) {
	t.Parallel()
	// Empty raw state: no LBs, so no diagnostic entries at all. The probe
	// fails (key absent => false) but no diagnostic is appended.
	raw := mustMarshal(t, map[string]any{})
	checks := []TopologyCheck{
		{Type: "http_probe", Target: "load_balancer", Port: 80, Expect: "reachable"},
	}

	failures, err := EvaluateTopology(raw, checks)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d (%+v)", len(failures), failures)
	}
	got := failures[0].Detail
	want := `http probe "load_balancer:80" expected true got false`
	if got != want {
		t.Errorf("expected bare detail %q, got %q", want, got)
	}
}

func TestEvaluateTopology_HTTPProbePassNoDetail(t *testing.T) {
	t.Parallel()
	// Healthy web app: probe on 80 is reachable -> no failure produced.
	raw := mustMarshal(t, webAppState())
	checks := []TopologyCheck{
		{Type: "http_probe", Target: "load_balancer", Port: 80, Expect: "reachable"},
	}

	failures, err := EvaluateTopology(raw, checks)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected no failures, got %d (%+v)", len(failures), failures)
	}
}

func TestEvaluateTopology_PreComputedTopologyHasNoDiagnostic(t *testing.T) {
	t.Parallel()
	// Pre-computed topology path: caller populated http_probe directly so
	// DeriveTopology never runs and no diagnostics are available. The
	// failure detail must remain the bare "expected/got" message — the
	// diagnostic plumbing must not regress this path.
	preComputed := map[string]any{
		"connectivity": map[string]any{},
		"http_probe": map[string]any{
			"load_balancer:80": false,
		},
	}
	raw := mustMarshal(t, preComputed)
	checks := []TopologyCheck{
		{Type: "http_probe", Target: "load_balancer", Port: 80, Expect: "reachable"},
	}

	failures, err := EvaluateTopology(raw, checks)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d (%+v)", len(failures), failures)
	}
	got := failures[0].Detail
	want := `http probe "load_balancer:80" expected true got false`
	if got != want {
		t.Errorf("expected bare pre-computed detail %q, got %q", want, got)
	}
	if strings.Contains(got, ":") && strings.Count(got, ":") > 1 {
		// The bare message has exactly one colon (inside the quoted key).
		// More than one would indicate a diagnostic suffix leaked in.
		// Defensive: this check is somewhat redundant with the equality
		// above but documents intent.
		t.Errorf("did not expect a diagnostic suffix in pre-computed path, got %q", got)
	}
}
