package generator

import (
	"encoding/json"
	"fmt"
)

// ExtractResourceTypesFromArchitecturePlan parses the phase 1 JSON output and
// returns a deduplicated list of resource type strings (e.g.
// ["scaleway_instance_server", "scaleway_vpc"]).
//
// The LLM sometimes wraps the JSON in explanatory prose, so this function
// attempts to extract a JSON object from the response if direct parsing fails.
func ExtractResourceTypesFromArchitecturePlan(planJSON string) ([]string, error) {
	jsonStr := planJSON
	var plan struct {
		Resources []struct {
			Type string `json:"type"`
		} `json:"resources"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		// Try to extract the JSON object from prose-wrapped output.
		if extracted, ok := extractJSONObject(jsonStr); ok {
			if err2 := json.Unmarshal([]byte(extracted), &plan); err2 != nil {
				return nil, fmt.Errorf("parse architecture plan JSON: %w", err)
			}
		} else {
			return nil, fmt.Errorf("parse architecture plan JSON: %w", err)
		}
	}

	seen := make(map[string]struct{}, len(plan.Resources))
	types := make([]string, 0, len(plan.Resources))
	for _, r := range plan.Resources {
		if r.Type == "" {
			continue
		}
		if _, ok := seen[r.Type]; ok {
			continue
		}
		seen[r.Type] = struct{}{}
		types = append(types, r.Type)
	}
	return types, nil
}

// extractJSONObject finds the first top-level JSON object in a string by
// locating the first '{' and its matching '}'. Returns the substring and true
// if found, or ("", false) otherwise.
func extractJSONObject(s string) (string, bool) {
	start := -1
	depth := 0
	inString := false
	escaped := false
	for i, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '{' {
			if start == -1 {
				start = i
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && start != -1 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}

// providerSchema mirrors the structure of `tofu providers schema -json` output
// just enough to extract and filter resource schemas.
type providerSchema struct {
	ProviderSchemas map[string]struct {
		ResourceSchemas map[string]json.RawMessage `json:"resource_schemas"`
	} `json:"provider_schemas"`
}

// companionResourceTypes maps resource types to related types that should
// always be included in the filtered schema when the key type is present.
// This prevents the LLM from removing required companion resources because
// they don't appear in the filtered schema.
var companionResourceTypes = map[string][]string{
	"scaleway_instance_server": {"scaleway_instance_private_nic"},
	"scaleway_k8s_cluster":    {"scaleway_k8s_pool"},
}

// FilterSchemaForResourceTypes parses the full tofu providers schema JSON and
// returns a compact JSON string containing only the resource_schemas entries
// whose keys match the given resource types (plus any companion types).
func FilterSchemaForResourceTypes(fullSchema []byte, resourceTypes []string) (string, error) {
	if len(resourceTypes) == 0 {
		return "", nil
	}

	var schema providerSchema
	if err := json.Unmarshal(fullSchema, &schema); err != nil {
		return "", fmt.Errorf("parse provider schema JSON: %w", err)
	}

	wanted := make(map[string]struct{}, len(resourceTypes))
	for _, rt := range resourceTypes {
		wanted[rt] = struct{}{}
		for _, companion := range companionResourceTypes[rt] {
			wanted[companion] = struct{}{}
		}
	}

	filtered := make(map[string]json.RawMessage)
	for _, ps := range schema.ProviderSchemas {
		for name, rs := range ps.ResourceSchemas {
			if _, ok := wanted[name]; ok {
				filtered[name] = rs
			}
		}
	}

	if len(filtered) == 0 {
		return "", nil
	}

	out, err := json.Marshal(filtered)
	if err != nil {
		return "", fmt.Errorf("marshal filtered schema: %w", err)
	}
	return string(out), nil
}
