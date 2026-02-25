package generator

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractResourceTypesFromArchitecturePlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "valid plan with multiple resources",
			input: `{"region":"fr-par","resources":[{"type":"scaleway_vpc","name":"main"},{"type":"scaleway_instance_server","name":"web"},{"type":"scaleway_vpc","name":"dup"}]}`,
			want:  []string{"scaleway_vpc", "scaleway_instance_server"},
		},
		{
			name:  "empty resources",
			input: `{"region":"fr-par","resources":[]}`,
			want:  []string{},
		},
		{
			name:  "no resources key",
			input: `{"region":"fr-par"}`,
			want:  []string{},
		},
		{
			name:    "malformed JSON",
			input:   `{invalid`,
			wantErr: true,
		},
		{
			name:  "resource with empty type is skipped",
			input: `{"resources":[{"type":"","name":"x"},{"type":"scaleway_vpc","name":"y"}]}`,
			want:  []string{"scaleway_vpc"},
		},
		{
			name:  "prose-wrapped JSON is extracted",
			input: "Looking at the failures, here is my plan:\n\n{\"resources\":[{\"type\":\"scaleway_vpc\",\"name\":\"main\"}]}",
			want:  []string{"scaleway_vpc"},
		},
		{
			name:    "pure prose with no JSON object",
			input:   "This is just prose without any JSON content.",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ExtractResourceTypesFromArchitecturePlan(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d types, got %d: %v", len(tc.want), len(got), got)
			}
			for i, w := range tc.want {
				if got[i] != w {
					t.Fatalf("type[%d]: expected %q, got %q", i, w, got[i])
				}
			}
		})
	}
}

func TestFilterSchemaForResourceTypes(t *testing.T) {
	t.Parallel()

	fullSchema := `{
		"provider_schemas": {
			"scaleway/scaleway": {
				"resource_schemas": {
					"scaleway_instance_server": {"block":{"attributes":{"name":{}}}},
					"scaleway_instance_private_nic": {"block":{"attributes":{"server_id":{}}}},
					"scaleway_vpc": {"block":{"attributes":{"name":{}}}},
					"scaleway_rdb_instance": {"block":{"attributes":{"name":{}}}},
					"scaleway_k8s_cluster": {"block":{"attributes":{"name":{}}}},
					"scaleway_k8s_pool": {"block":{"attributes":{"cluster_id":{}}}}
				}
			}
		}
	}`

	tests := []struct {
		name      string
		schema    string
		types     []string
		wantKeys  []string
		wantEmpty bool
		wantErr   bool
	}{
		{
			name:     "matching types",
			schema:   fullSchema,
			types:    []string{"scaleway_instance_server", "scaleway_vpc"},
			wantKeys: []string{"scaleway_instance_server", "scaleway_vpc", "scaleway_instance_private_nic"},
		},
		{
			name:     "companion type included automatically",
			schema:   fullSchema,
			types:    []string{"scaleway_instance_server"},
			wantKeys: []string{"scaleway_instance_server", "scaleway_instance_private_nic"},
		},
		{
			name:     "k8s companion type included automatically",
			schema:   fullSchema,
			types:    []string{"scaleway_k8s_cluster"},
			wantKeys: []string{"scaleway_k8s_cluster", "scaleway_k8s_pool"},
		},
		{
			name:      "no matches",
			schema:    fullSchema,
			types:     []string{"scaleway_nonexistent"},
			wantEmpty: true,
		},
		{
			name:      "empty types list",
			schema:    fullSchema,
			types:     []string{},
			wantEmpty: true,
		},
		{
			name:      "nil types list",
			schema:    fullSchema,
			types:     nil,
			wantEmpty: true,
		},
		{
			name:    "invalid schema JSON",
			schema:  `{broken`,
			types:   []string{"scaleway_vpc"},
			wantErr: true,
		},
		{
			name:      "empty provider_schemas",
			schema:    `{"provider_schemas":{}}`,
			types:     []string{"scaleway_vpc"},
			wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := FilterSchemaForResourceTypes([]byte(tc.schema), tc.types)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantEmpty {
				if got != "" {
					t.Fatalf("expected empty result, got %q", got)
				}
				return
			}

			var parsed map[string]json.RawMessage
			if err := json.Unmarshal([]byte(got), &parsed); err != nil {
				t.Fatalf("result is not valid JSON: %v", err)
			}
			if len(parsed) != len(tc.wantKeys) {
				t.Fatalf("expected %d keys, got %d: %v", len(tc.wantKeys), len(parsed), keysOf(parsed))
			}
			for _, key := range tc.wantKeys {
				if _, ok := parsed[key]; !ok {
					t.Fatalf("expected key %q in result", key)
				}
			}

			// Verify unwanted resources are excluded.
			if _, ok := parsed["scaleway_rdb_instance"]; ok && !contains(tc.types, "scaleway_rdb_instance") {
				t.Fatal("expected scaleway_rdb_instance to be excluded")
			}
		})
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
