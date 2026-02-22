package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/open-policy-agent/opa/rego"
	"github.com/redscaresu/infrafactory/internal/feedback"
)

func EvaluateStatePolicies(ctx context.Context, stateJSON []byte, policyPaths []string) ([]feedback.Failure, error) {
	return EvaluateStatePoliciesWithInput(ctx, stateJSON, nil, policyPaths)
}

func EvaluateStatePoliciesWithInput(ctx context.Context, stateJSON []byte, extraInput map[string]any, policyPaths []string) ([]feedback.Failure, error) {
	if len(policyPaths) == 0 {
		return nil, nil
	}

	var decoded any
	if err := json.Unmarshal(stateJSON, &decoded); err != nil {
		return nil, fmt.Errorf("decode state json: %w", err)
	}
	input := decoded
	if len(extraInput) > 0 {
		if stateMap, ok := decoded.(map[string]any); ok {
			envelope := make(map[string]any, len(stateMap)+len(extraInput)+1)
			for key, value := range stateMap {
				envelope[key] = value
			}
			envelope["state"] = stateMap
			for key, value := range extraInput {
				envelope[key] = value
			}
			input = envelope
		} else {
			envelope := make(map[string]any, len(extraInput)+1)
			envelope["state"] = decoded
			for key, value := range extraInput {
				envelope[key] = value
			}
			input = envelope
		}
	}

	packages, err := discoverPolicyPackages(policyPaths)
	if err != nil {
		return nil, err
	}
	failures := make([]feedback.Failure, 0)
	for _, pkg := range packages {
		query, err := rego.New(
			rego.Query(fmt.Sprintf("data.%s.deny_state", pkg)),
			rego.Load(policyPaths, nil),
			rego.Input(input),
		).Eval(ctx)
		if err != nil {
			return nil, fmt.Errorf("evaluate state policy package %q: %w", pkg, err)
		}
		if len(query) == 0 || len(query[0].Expressions) == 0 {
			continue
		}

		for _, detail := range denyMessages(query[0].Expressions[0].Value) {
			failures = append(failures, feedback.Failure{
				Layer:   "mock_deploy",
				Stage:   "state_policy",
				Status:  "fail",
				Check:   "policy",
				Policy:  pkg,
				Command: "opa eval",
				Detail:  detail,
			})
		}
	}

	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Policy == failures[j].Policy {
			return failures[i].Detail < failures[j].Detail
		}
		return failures[i].Policy < failures[j].Policy
	})

	return failures, nil
}
