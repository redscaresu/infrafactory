package harness

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type rawMockState struct {
	Instance struct {
		Servers     []map[string]any `json:"servers"`
		IPs         []map[string]any `json:"ips"`
		PrivateNICs []map[string]any `json:"private_nics"`
	} `json:"instance"`
	LB struct {
		LBs       []map[string]any `json:"lbs"`
		IPs       []map[string]any `json:"ips"`
		Frontends []map[string]any `json:"frontends"`
		Backends  []map[string]any `json:"backends"`
	} `json:"lb"`
	VPC struct {
		PrivateNetworks []map[string]any `json:"private_networks"`
	} `json:"vpc"`
	RDB struct {
		Instances []map[string]any `json:"instances"`
	} `json:"rdb"`
	Redis struct {
		Clusters []map[string]any `json:"clusters"`
	} `json:"redis"`
}

// DeriveTopology computes connectivity and http_probe maps from raw mock
// resource state. It auto-detects the cloud provider from the state shape
// (mockway uses `instance/lb/rdb/...` keys; fakegcp uses
// `compute/container/sql/lb`) and dispatches to the appropriate
// per-cloud derivation. Returns JSON with the same shape that
// EvaluateTopology expects, plus a Diagnostics map that explains why
// http_probe entries are false. Diagnostics is non-nil and may be empty.
//
// Diagnostics keys:
//   - For each false http_probe entry, a key matching the http_probe key
//     (e.g. "load_balancer:80") with a specific reason (no backend, no ip).
//   - For each load balancer, a fallback key "load_balancer" describing the
//     overall LB state (no frontends, frontends on other ports, etc.). This
//     lets consumers explain probes that hit ports no frontend listens on,
//     since at derivation time the requested port is not yet known.
func DeriveTopology(stateJSON []byte) ([]byte, map[string]string, error) {
	switch detectCloud(stateJSON) {
	case "gcp":
		return deriveTopologyGCP(stateJSON)
	case "aws":
		return deriveTopologyAWS(stateJSON)
	case "genesys":
		return deriveTopologyGenesys(stateJSON)
	default:
		return deriveTopologyScaleway(stateJSON)
	}
}

// detectCloud inspects the raw mock state to decide which cloud
// produced it. mockway (Scaleway): `instance`/`lb`/`rdb` keys.
// fakegcp: top-level `compute` key. fakeaws: schema_version=1 with
// top-level `iam` AND `s3` blocks (per fakeaws/handlers/admin.go).
// Defaults to "scaleway" so empty/unknown state keeps the historical
// behaviour.
func detectCloud(stateJSON []byte) string {
	var probe struct {
		Compute       json.RawMessage `json:"compute"`
		Instance      json.RawMessage `json:"instance"`
		IAM           json.RawMessage `json:"iam"`
		S3            json.RawMessage `json:"s3"`
		SchemaVersion json.RawMessage `json:"schema_version"`
		// fakegenesys (S114-T5): top-level `routing_queues` + `flows`
		// keys distinguish it from fakeaws (no routing_queues key).
		RoutingQueues json.RawMessage `json:"routing_queues"`
		Flows         json.RawMessage `json:"flows"`
	}
	if err := json.Unmarshal(stateJSON, &probe); err != nil {
		return "scaleway"
	}
	// Genesys: schema_version + routing_queues + flows present. Check
	// BEFORE the AWS probe so we don't misclassify when both have
	// schema_version=1.
	genesysLike := len(probe.SchemaVersion) > 0 &&
		len(probe.RoutingQueues) > 0 && string(probe.RoutingQueues) != "null" &&
		len(probe.Flows) > 0 && string(probe.Flows) != "null"
	if genesysLike {
		return "genesys"
	}
	// AWS: schema_version present AND both iam + s3 blocks are present.
	// We require all three to disambiguate from fakegcp's `iam` key.
	awsLike := len(probe.SchemaVersion) > 0 &&
		len(probe.S3) > 0 && string(probe.S3) != "null" &&
		len(probe.IAM) > 0 && string(probe.IAM) != "null"
	if awsLike {
		return "aws"
	}
	if len(probe.Compute) > 0 && string(probe.Compute) != "null" {
		return "gcp"
	}
	return "scaleway"
}

// deriveTopologyAWS is the AWS-specific topology emitter. At S43-T9
// this returns an empty-but-valid topology (IAM + S3 don't
// contribute to the connectivity/probe graph). Service tickets in
// S44+ extend this when EC2/RDS/EKS land — that's where load_balancer
// probes and connectivity entries get populated.
//
// Returns (topologyJSON, diagnostics, error). Diagnostics is non-nil
// (may be empty) for parity with the GCP/Scaleway derivers.
func deriveTopologyAWS(stateJSON []byte) ([]byte, map[string]string, error) {
	out := map[string]any{
		"http_probe":   map[string]any{},
		"connectivity": map[string]any{},
	}
	body, err := json.Marshal(out)
	if err != nil {
		return nil, nil, err
	}
	return body, map[string]string{}, nil
}

func deriveTopologyScaleway(stateJSON []byte) ([]byte, map[string]string, error) {
	var state rawMockState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, nil, fmt.Errorf("unmarshal raw state: %w", err)
	}

	connectivity := deriveConnectivity(&state)
	httpProbe, diagnostics := deriveHTTPProbe(&state)

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

// deriveHTTPProbe returns the http_probe boolean map plus a diagnostics map
// that records, for every false entry, the specific reason the probe is false.
// Diagnostic strings are short, lowercase, and factual so callers (S35-T2) can
// embed them directly in user-facing failure messages.
func deriveHTTPProbe(state *rawMockState) (map[string]bool, map[string]string) {
	probe := make(map[string]bool)
	diagnostics := make(map[string]string)

	for _, lb := range state.LB.LBs {
		lbID := jsonStr(lb, "id")
		if lbID == "" {
			continue
		}

		hasBackend := false
		for _, backend := range state.LB.Backends {
			if jsonStr(backend, "lb_id") == lbID {
				hasBackend = true
				break
			}
		}

		hasIP := false
		for _, ip := range state.LB.IPs {
			if jsonStr(ip, "lb_id") == lbID {
				hasIP = true
				break
			}
		}

		frontendPorts := make([]int, 0)
		for _, frontend := range state.LB.Frontends {
			if jsonStr(frontend, "lb_id") != lbID {
				continue
			}
			port := jsonInt(frontend["inbound_port"])
			if port == 0 {
				continue
			}
			frontendPorts = append(frontendPorts, port)
		}
		sort.Ints(frontendPorts)

		// Per-frontend probe entries.
		for _, port := range frontendPorts {
			key := httpProbeKey("load_balancer", port)
			reachable := hasBackend && hasIP
			probe[key] = reachable
			if !reachable {
				diagnostics[key] = httpProbeReason(hasBackend, hasIP)
			}
		}

		// LB-level fallback diagnostic, used when a probe targets a port no
		// frontend listens on (S35-T2 looks this up when the port-specific
		// key is absent from http_probe).
		if msg := lbFallbackReason(frontendPorts, hasBackend, hasIP); msg != "" {
			diagnostics[httpProbeFallbackKey()] = msg
		}
	}

	return probe, diagnostics
}

// httpProbeReason picks the most specific reason a probe is unreachable for a
// frontend that does exist on the requested port.
func httpProbeReason(hasBackend, hasIP bool) string {
	switch {
	case !hasBackend && !hasIP:
		return "no backend attached and no public ip on lb"
	case !hasBackend:
		return "no backend attached"
	case !hasIP:
		return "no public ip on lb"
	default:
		return "unknown"
	}
}

// lbFallbackReason returns a diagnostic for the LB as a whole, used when a
// probe targets a port no frontend listens on. Returns "" when nothing useful
// can be said (LB is fully wired and frontends exist; the port-specific
// diagnostic alone covers it).
func lbFallbackReason(frontendPorts []int, hasBackend, hasIP bool) string {
	if len(frontendPorts) == 0 {
		switch {
		case !hasIP && !hasBackend:
			return "no frontends, no backend, no public ip on lb"
		case !hasIP:
			return "no frontends and no public ip on lb"
		case !hasBackend:
			return "no frontends and no backend attached"
		default:
			return "no frontends configured on lb"
		}
	}
	parts := make([]string, len(frontendPorts))
	for i, p := range frontendPorts {
		parts[i] = strconv.Itoa(p)
	}
	listing := strings.Join(parts, ",")
	// Even with backends and an IP, the fallback is still useful: a probe
	// on a port no frontend listens on would otherwise have no diagnostic
	// at all. Suppressing it only when *every* observable issue is absent
	// (i.e. a fully healthy LB and the probe targeted an existing port)
	// is impossible at derivation time, so always include the fallback
	// when frontends exist and let the consumer decide whether to use it.
	switch {
	case !hasIP && !hasBackend:
		return fmt.Sprintf("frontends on port %s, no backend attached, no public ip on lb", listing)
	case !hasIP:
		return fmt.Sprintf("frontends on port %s, no public ip on lb", listing)
	case !hasBackend:
		return fmt.Sprintf("frontends on port %s, no backend attached", listing)
	default:
		return fmt.Sprintf("frontends on port %s", listing)
	}
}

// httpProbeFallbackKey returns the diagnostic key used when no port-specific
// http_probe entry exists for a load balancer. It deliberately matches the
// "target" portion of an http_probe key so S35-T2 can derive it from a
// failing check.
func httpProbeFallbackKey() string {
	return "load_balancer"
}

func deriveConnectivity(state *rawMockState) map[string]bool {
	conn := make(map[string]bool)

	// Collect all private network IDs that servers are connected to.
	serverPNs := make(map[string]bool)
	for _, nic := range state.Instance.PrivateNICs {
		pnID := jsonStr(nic, "private_network_id")
		if pnID != "" {
			serverPNs[pnID] = true
		}
	}

	// RDB connectivity.
	for _, rdb := range state.RDB.Instances {
		endpoints, ok := rdb["endpoints"].([]any)
		if !ok {
			continue
		}
		for _, epRaw := range endpoints {
			ep, ok := epRaw.(map[string]any)
			if !ok {
				continue
			}
			port := jsonInt(ep["port"])
			if port == 0 {
				continue
			}

			pn, hasPNKey := ep["private_network"]
			if hasPNKey && pn != nil {
				pnMap, ok := pn.(map[string]any)
				if ok {
					pnID := jsonStr(pnMap, "id")
					if pnID != "" && serverPNs[pnID] {
						key := connectivityKey("compute", "database", port)
						conn[key] = true
					}
				}
			} else {
				// Public endpoint: no private_network key or nil value.
				key := connectivityKey("public_internet", "database", port)
				conn[key] = true
			}
		}
	}

	// Redis connectivity.
	for _, cluster := range state.Redis.Clusters {
		endpoints, ok := cluster["endpoints"].([]any)
		if !ok {
			continue
		}
		for _, epRaw := range endpoints {
			ep, ok := epRaw.(map[string]any)
			if !ok {
				continue
			}
			port := jsonInt(ep["port"])
			if port == 0 {
				continue
			}
			pn, hasPNKey := ep["private_network"]
			if hasPNKey && pn != nil {
				pnMap, ok := pn.(map[string]any)
				if ok {
					pnID := jsonStr(pnMap, "id")
					if pnID != "" && serverPNs[pnID] {
						key := connectivityKey("compute", "redis", port)
						conn[key] = true
					}
				}
			}
		}
	}

	// Public internet → compute: check if any server has a public IP.
	for _, ip := range state.Instance.IPs {
		serverObj, ok := ip["server"].(map[string]any)
		if !ok {
			continue
		}
		serverID := jsonStr(serverObj, "id")
		if serverID != "" {
			key := connectivityKey("public_internet", "compute", 0)
			conn[key] = true
			break
		}
	}

	return conn
}

// jsonStr safely extracts a string value from a map.
func jsonStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// jsonInt converts a JSON number (float64) to int.
func jsonInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
