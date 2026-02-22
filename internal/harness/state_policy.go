package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/open-policy-agent/opa/rego"
	"github.com/redscaresu/infrafactory/internal/feedback"
)

func EvaluateStatePolicies(ctx context.Context, stateJSON []byte, policyPaths []string) ([]feedback.Failure, error) {
	if len(policyPaths) == 0 {
		return nil, nil
	}

	var input any
	if err := json.Unmarshal(stateJSON, &input); err != nil {
		return nil, fmt.Errorf("decode state json: %w", err)
	}

	query, err := rego.New(
		rego.Query("data"),
		rego.Load(policyPaths, nil),
		rego.Input(input),
	).Eval(ctx)
	if err != nil {
		return nil, fmt.Errorf("evaluate state policy: %w", err)
	}
	if len(query) == 0 || len(query[0].Expressions) == 0 {
		return nil, nil
	}

	root, ok := query[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return nil, nil
	}

	failures := make([]feedback.Failure, 0)
	collectStateDenyFailures(root, nil, &failures)
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Policy == failures[j].Policy {
			return failures[i].Detail < failures[j].Detail
		}
		return failures[i].Policy < failures[j].Policy
	})

	return failures, nil
}

func collectStateDenyFailures(node any, path []string, out *[]feedback.Failure) {
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			if key == "deny_state" {
				policy := strings.Join(path, ".")
				for _, detail := range denyMessages(value) {
					*out = append(*out, feedback.Failure{
						Layer:   "mock_deploy",
						Stage:   "state_policy",
						Status:  "fail",
						Check:   "policy",
						Policy:  policy,
						Command: "opa eval",
						Detail:  detail,
					})
				}
				continue
			}

			collectStateDenyFailures(value, append(path, key), out)
		}
	}
}
