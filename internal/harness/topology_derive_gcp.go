package harness

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// rawGCPState mirrors the per-service shape of fakegcp's `/mock/state`
// response. Only the fields infrafactory's topology evaluation cares about
// are pulled out; unknown fields are ignored. See `../fakegcp/repository`
// for the canonical shape.
type rawGCPState struct {
	Compute struct {
		Networks    []map[string]any `json:"networks"`
		Subnetworks []map[string]any `json:"subnetworks"`
		Firewalls   []map[string]any `json:"firewalls"`
		Instances   []map[string]any `json:"instances"`
	} `json:"compute"`
	Container struct {
		Clusters  []map[string]any `json:"clusters"`
		NodePools []map[string]any `json:"nodePools"`
	} `json:"container"`
	SQL struct {
		Instances []map[string]any `json:"instances"`
	} `json:"sql"`
	LB struct {
		GlobalForwardingRules []map[string]any `json:"global_forwarding_rules"`
		BackendServices       []map[string]any `json:"backend_services"`
	} `json:"lb"`
}

// deriveTopologyGCP turns fakegcp's raw service state into the same
// connectivity + http_probe + diagnostics shape that the Scaleway path
// produces, so EvaluateTopology stays cloud-agnostic. The supported
// resource types are the four called out in S36-T4:
//
//   - google_compute_instance         → compute → compute connectivity
//   - google_compute_forwarding_rule  → load_balancer:port http_probe
//   - google_sql_database_instance    → compute → database connectivity
//   - google_container_cluster        → compute → kubernetes connectivity
//
// The connectivity model is intentionally permissive on the private side:
// any two compute-on-private-subnetwork resources are treated as mutually
// reachable; public-internet → compute is denied unless a 0.0.0.0/0 firewall
// rule explicitly allows it. This mirrors the Scaleway derivation's
// pragmatism — it's enough to detect the gross "is this scenario wired up
// at all" failures the topology criteria are written to catch.
func deriveTopologyGCP(stateJSON []byte) ([]byte, map[string]string, error) {
	var state rawGCPState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, nil, fmt.Errorf("unmarshal raw gcp state: %w", err)
	}

	connectivity := deriveGCPConnectivity(&state)
	httpProbe, diagnostics := deriveGCPHTTPProbe(&state)

	result := map[string]any{
		"connectivity": connectivity,
		"http_probe":   httpProbe,
	}
	out, err := json.Marshal(result)
	if err != nil {
		return nil, nil, err
	}
	return out, diagnostics, nil
}

func deriveGCPHTTPProbe(state *rawGCPState) (map[string]bool, map[string]string) {
	probe := make(map[string]bool)
	diagnostics := make(map[string]string)

	hasBackend := false
	for _, bs := range state.LB.BackendServices {
		if backends, ok := bs["backends"].([]any); ok && len(backends) > 0 {
			hasBackend = true
			break
		}
	}

	if len(state.LB.GlobalForwardingRules) == 0 {
		// Nothing to probe; consumers without an LB will get an empty
		// http_probe map and EvaluateTopology will fall back to the bare
		// "expected reachable, got unreachable" message.
		return probe, diagnostics
	}

	frontendPorts := make(map[int]struct{})
	for _, fr := range state.LB.GlobalForwardingRules {
		port := gcpForwardingRulePort(fr)
		if port == 0 {
			continue
		}
		frontendPorts[port] = struct{}{}
		key := fmt.Sprintf("load_balancer:%d", port)
		reachable := hasBackend
		probe[key] = reachable
		if !reachable {
			diagnostics[key] = "no backend services with backends attached"
		}
	}

	// LB-level fallback diagnostic mirrors the Scaleway path so
	// EvaluateTopology can explain probes whose port has no entry above.
	if len(frontendPorts) == 0 {
		diagnostics["load_balancer"] = "no forwarding rules configured"
	} else if !hasBackend {
		diagnostics["load_balancer"] = "no backend services with backends attached"
	} else {
		ports := make([]int, 0, len(frontendPorts))
		for p := range frontendPorts {
			ports = append(ports, p)
		}
		sortInts(ports)
		strs := make([]string, len(ports))
		for i, p := range ports {
			strs[i] = strconv.Itoa(p)
		}
		diagnostics["load_balancer"] = "forwarding rules on port " + strings.Join(strs, ",")
	}

	return probe, diagnostics
}

func deriveGCPConnectivity(state *rawGCPState) map[string]bool {
	conn := make(map[string]bool)

	hasCompute := len(state.Compute.Instances) > 0
	hasDatabase := len(state.SQL.Instances) > 0
	hasKubernetes := len(state.Container.Clusters) > 0

	publicIngressAllowed := gcpHasPublicIngressFirewall(state.Compute.Firewalls)

	if hasCompute && hasDatabase {
		conn["compute->database:5432"] = true
		conn["compute->database:3306"] = true
		conn["public_internet->database:5432"] = false
		conn["public_internet->database:3306"] = false
	}
	if hasCompute && hasKubernetes {
		conn["compute->kubernetes"] = true
	}
	if hasCompute {
		conn["public_internet->compute"] = publicIngressAllowed
	}

	return conn
}

// gcpForwardingRulePort extracts the listening port from a global
// forwarding rule entry. fakegcp/Terraform encodes it either via a single
// `port_range` ("80" or "80-80") or a `ports` array; we accept both.
func gcpForwardingRulePort(fr map[string]any) int {
	if pr, ok := fr["port_range"].(string); ok && pr != "" {
		if port := parseLeadingInt(pr); port > 0 {
			return port
		}
	}
	if ports, ok := fr["ports"].([]any); ok && len(ports) > 0 {
		switch v := ports[0].(type) {
		case string:
			if port := parseLeadingInt(v); port > 0 {
				return port
			}
		case float64:
			return int(v)
		}
	}
	return 0
}

// gcpHasPublicIngressFirewall reports whether any firewall rule allows
// ingress from 0.0.0.0/0 — the canonical GCP marker for a public-facing
// instance. Only ALLOW rules with INGRESS direction count.
func gcpHasPublicIngressFirewall(firewalls []map[string]any) bool {
	for _, fw := range firewalls {
		direction, _ := fw["direction"].(string)
		if direction != "" && !strings.EqualFold(direction, "INGRESS") {
			continue
		}
		ranges, ok := fw["source_ranges"].([]any)
		if !ok {
			continue
		}
		for _, r := range ranges {
			if s, _ := r.(string); s == "0.0.0.0/0" {
				if hasGCPAllowRule(fw) {
					return true
				}
			}
		}
	}
	return false
}

func hasGCPAllowRule(fw map[string]any) bool {
	allowed, ok := fw["allowed"].([]any)
	if !ok {
		return false
	}
	return len(allowed) > 0
}

func parseLeadingInt(s string) int {
	s = strings.TrimSpace(s)
	if dash := strings.IndexByte(s, '-'); dash > 0 {
		s = s[:dash]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func sortInts(in []int) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j-1] > in[j]; j-- {
			in[j-1], in[j] = in[j], in[j-1]
		}
	}
}
