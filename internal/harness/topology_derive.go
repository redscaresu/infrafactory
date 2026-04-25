package harness

import (
	"encoding/json"
	"fmt"
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

// DeriveTopology computes connectivity and http_probe maps from raw mockway
// resource state. Returns JSON with the same shape that EvaluateTopology expects.
func DeriveTopology(stateJSON []byte) ([]byte, error) {
	var state rawMockState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, fmt.Errorf("unmarshal raw state: %w", err)
	}

	connectivity := deriveConnectivity(&state)
	httpProbe := deriveHTTPProbe(&state)

	result := map[string]any{
		"connectivity": connectivity,
		"http_probe":   httpProbe,
	}
	return json.Marshal(result)
}

func deriveHTTPProbe(state *rawMockState) map[string]bool {
	probe := make(map[string]bool)

	for _, lb := range state.LB.LBs {
		lbID := jsonStr(lb, "id")
		if lbID == "" {
			continue
		}

		// Check if LB has at least one backend.
		hasBackend := false
		for _, backend := range state.LB.Backends {
			if jsonStr(backend, "lb_id") == lbID {
				hasBackend = true
				break
			}
		}

		// Check if LB has an IP. First check the lb_ips table, then fall
		// back to the LB's embedded "ip" array (mockway may not persist
		// lb_id back to the lb_ips table in all cases).
		hasIP := false
		for _, ip := range state.LB.IPs {
			if jsonStr(ip, "lb_id") == lbID {
				hasIP = true
				break
			}
		}
		if !hasIP {
			if ipArr, ok := lb["ip"].([]any); ok && len(ipArr) > 0 {
				hasIP = true
			}
		}

		// For each frontend on this LB, set http_probe key.
		for _, frontend := range state.LB.Frontends {
			if jsonStr(frontend, "lb_id") != lbID {
				continue
			}
			port := jsonInt(frontend["inbound_port"])
			if port == 0 {
				continue
			}
			key := httpProbeKey("load_balancer", port)
			probe[key] = hasBackend && hasIP
		}
	}

	return probe
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
