package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/open-policy-agent/opa/rego"
	"github.com/redscaresu/infrafactory/internal/feedback"
)

func EvaluatePlanPolicies(ctx context.Context, planJSON []byte, policyPaths []string) ([]feedback.Failure, error) {
	return EvaluatePlanPoliciesWithConstraints(ctx, planJSON, nil, policyPaths)
}

func EvaluatePlanPoliciesWithConstraints(ctx context.Context, planJSON []byte, constraints map[string]any, policyPaths []string) ([]feedback.Failure, error) {
	if len(policyPaths) == 0 {
		return nil, nil
	}

	var decoded any
	if err := json.Unmarshal(planJSON, &decoded); err != nil {
		return nil, fmt.Errorf("decode plan json: %w", err)
	}
	input := decoded
	if len(constraints) > 0 {
		if planMap, ok := decoded.(map[string]any); ok {
			envelope := make(map[string]any, len(planMap)+1)
			for key, value := range planMap {
				envelope[key] = value
			}
			envelope["constraints"] = constraints
			input = envelope
		} else {
			input = map[string]any{
				"plan":        decoded,
				"constraints": constraints,
			}
		}
	}

	packages, err := discoverPolicyPackages(policyPaths)
	if err != nil {
		return nil, err
	}
	failures := make([]feedback.Failure, 0)
	for _, pkg := range packages {
		query, err := rego.New(
			rego.Query(fmt.Sprintf("data.%s.deny", pkg)),
			rego.Load(policyPaths, nil),
			rego.Input(input),
		).Eval(ctx)
		if err != nil {
			return nil, fmt.Errorf("evaluate policy package %q: %w", pkg, err)
		}
		if len(query) == 0 || len(query[0].Expressions) == 0 {
			continue
		}

		for _, detail := range denyMessages(query[0].Expressions[0].Value) {
			failures = append(failures, feedback.Failure{
				Layer:   "static",
				Stage:   "opa",
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

func denyMessages(deny any) []string {
	switch typed := deny.(type) {
	case []any:
		messages := make([]string, 0, len(typed))
		for _, value := range typed {
			messages = append(messages, fmt.Sprint(value))
		}
		sort.Strings(messages)
		return messages
	case map[string]any:
		messages := make([]string, 0, len(typed))
		for key := range typed {
			messages = append(messages, key)
		}
		sort.Strings(messages)
		return messages
	default:
		return nil
	}
}
