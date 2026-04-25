package harness

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/redscaresu/infrafactory/internal/feedback"
)

type TopologyCheck struct {
	Type   string
	From   string
	To     string
	Target string
	Port   int
	Expect string
}

func EvaluateTopology(stateJSON []byte, checks []TopologyCheck) ([]feedback.Failure, error) {
	var state topologyState
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		return nil, fmt.Errorf("decode mock state: %w", err)
	}

	// If no pre-computed topology maps exist, derive from raw resource state.
	if state.Connectivity == nil && state.HTTPProbe == nil {
		derived, err := DeriveTopology(stateJSON)
		if err != nil {
			return nil, fmt.Errorf("derive topology: %w", err)
		}
		if err := json.Unmarshal(derived, &state); err != nil {
			return nil, fmt.Errorf("decode derived topology: %w", err)
		}
	}

	failures := make([]feedback.Failure, 0)
	for _, check := range checks {
		switch check.Type {
		case "connectivity":
			key := connectivityKey(check.From, check.To, check.Port)
			actual := state.Connectivity[key]
			expected := check.Expect == "success"
			if actual != expected {
				failures = append(failures, feedback.Failure{
					Layer:   "mock_deploy",
					Stage:   "topology",
					Status:  "fail",
					Check:   "connectivity",
					Command: "topology evaluator",
					Detail:  fmt.Sprintf("connectivity %q expected %t got %t", key, expected, actual),
				})
			}
		case "http_probe":
			key := httpProbeKey(check.Target, check.Port)
			actual := state.HTTPProbe[key]
			expected := check.Expect == "reachable"
			if actual != expected {
				failures = append(failures, feedback.Failure{
					Layer:   "mock_deploy",
					Stage:   "topology",
					Status:  "fail",
					Check:   "http_probe",
					Command: "topology evaluator",
					Detail:  fmt.Sprintf("http probe %q expected %t got %t", key, expected, actual),
				})
			}
		}
	}

	return failures, nil
}

type topologyState struct {
	Connectivity map[string]bool `json:"connectivity"`
	HTTPProbe    map[string]bool `json:"http_probe"`
}

func connectivityKey(from, to string, port int) string {
	key := from + "->" + to
	if port > 0 {
		return key + ":" + strconv.Itoa(port)
	}
	return key
}

func httpProbeKey(target string, port int) string {
	return target + ":" + strconv.Itoa(port)
}
