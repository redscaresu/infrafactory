package scenario

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const DefaultSchemaPath = "scenario.schema.json"

var (
	ErrInvalidScenario   = errors.New("invalid scenario")
	ErrMalformedScenario = errors.New("malformed scenario")
)

type Scenario struct {
	Name               string                 `json:"scenario"`
	Version            string                 `json:"version"`
	Cloud              string                 `json:"cloud"`
	Description        string                 `json:"description"`
	Type               string                 `json:"type,omitempty"`
	References         string                 `json:"references,omitempty"`
	Resources          Resources              `json:"resources"`
	Constraints        map[string]any         `json:"constraints,omitempty"`
	AcceptanceCriteria []AcceptanceCriterion  `json:"acceptance_criteria"`
}

type Resources struct {
	Compute    *ComputeResource    `json:"compute,omitempty"`
	Networking *NetworkingResource `json:"networking,omitempty"`
	Database   *DatabaseResource   `json:"database,omitempty"`
	Kubernetes *KubernetesResource `json:"kubernetes,omitempty"`
}

type ComputeResource struct {
	Purpose  string          `json:"purpose"`
	Size     string          `json:"size"`
	Count    int             `json:"count,omitempty"`
	Override ComputeOverride `json:"override,omitempty"`
}

type ComputeOverride struct {
	Offer string `json:"offer,omitempty"`
	Image string `json:"image,omitempty"`
}

type NetworkingResource struct {
	VPC            bool          `json:"vpc,omitempty"`
	PrivateNetwork bool          `json:"private_network,omitempty"`
	LoadBalancer   *LoadBalancer `json:"load_balancer,omitempty"`
}

type LoadBalancer struct {
	Exposure string       `json:"exposure"`
	Backends []LBBackend  `json:"backends"`
}

type LBBackend struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type DatabaseResource struct {
	Engine           string           `json:"engine"`
	Size             string           `json:"size"`
	HighAvailability bool             `json:"high_availability,omitempty"`
	Override         DatabaseOverride `json:"override,omitempty"`
}

type DatabaseOverride struct {
	NodeType      string `json:"node_type,omitempty"`
	EngineVersion string `json:"engine_version,omitempty"`
}

type KubernetesResource struct {
	Size     string             `json:"size"`
	Override KubernetesOverride `json:"override,omitempty"`
}

type KubernetesOverride struct {
	NodeType  string `json:"node_type,omitempty"`
	NodeCount int    `json:"node_count,omitempty"`
}

type AcceptanceCriterion struct {
	Type        string `json:"type"`
	Expect      string `json:"expect"`
	Description string `json:"description,omitempty"`

	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Port   *int   `json:"port,omitempty"`
	Target string `json:"target,omitempty"`
	Check  string `json:"check,omitempty"`
	Domain string `json:"domain,omitempty"`
}

type Violation struct {
	Path    string
	Message string
}

type ValidationError struct {
	ScenarioPath string
	Violations   []Violation
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Violations) == 0 {
		return ErrInvalidScenario.Error()
	}

	out := fmt.Sprintf("%s: %s", ErrInvalidScenario.Error(), e.ScenarioPath)
	for _, violation := range e.Violations {
		out = fmt.Sprintf("%s: %s %s", out, violation.Path, violation.Message)
	}

	return out
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidScenario
}

func Load(path string) (Scenario, error) {
	return LoadWithSchema(path, DefaultSchemaPath)
}

func LoadWithSchema(path, schemaPath string) (Scenario, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, fmt.Errorf("read scenario %q: %w", path, err)
	}

	var raw any
	if err := yaml.Unmarshal(payload, &raw); err != nil {
		return Scenario{}, fmt.Errorf("%w: parse scenario %q: %v", ErrMalformedScenario, path, err)
	}

	normalized := normalizeYAML(raw)

	schema, err := compileSchema(schemaPath)
	if err != nil {
		return Scenario{}, fmt.Errorf("compile scenario schema %q: %w", schemaPath, err)
	}

	if err := schema.Validate(normalized); err != nil {
		var schemaErr *jsonschema.ValidationError
		if errors.As(err, &schemaErr) {
			violations := flattenViolations(schemaErr)
			sort.Slice(violations, func(i, j int) bool {
				if violations[i].Path == violations[j].Path {
					return violations[i].Message < violations[j].Message
				}
				return violations[i].Path < violations[j].Path
			})
			return Scenario{}, &ValidationError{
				ScenarioPath: path,
				Violations:   violations,
			}
		}
		return Scenario{}, fmt.Errorf("%w: validate scenario %q: %v", ErrInvalidScenario, path, err)
	}

	jsonBytes, err := json.Marshal(normalized)
	if err != nil {
		return Scenario{}, fmt.Errorf("marshal scenario %q: %w", path, err)
	}

	var result Scenario
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return Scenario{}, fmt.Errorf("decode scenario %q: %w", path, err)
	}

	return result, nil
}

func compileSchema(schemaPath string) (*jsonschema.Schema, error) {
	absPath, err := filepath.Abs(schemaPath)
	if err != nil {
		return nil, err
	}

	compiler := jsonschema.NewCompiler()
	return compiler.Compile(absPath)
}

func flattenViolations(err *jsonschema.ValidationError) []Violation {
	if len(err.Causes) == 0 {
		path := formatPath(err.InstanceLocation)
		return []Violation{{
			Path:    path,
			Message: err.Error(),
		}}
	}

	violations := make([]Violation, 0)
	for _, cause := range err.Causes {
		violations = append(violations, flattenViolations(cause)...)
	}

	return violations
}

func formatPath(instanceLocation []string) string {
	if len(instanceLocation) == 0 {
		return "/"
	}

	path := ""
	for _, part := range instanceLocation {
		path = path + "/" + part
	}

	return path
}

func normalizeYAML(value any) any {
	// YAML decoders may produce map[any]any; convert recursively to map[string]any
	// so JSON schema validation and json.Unmarshal operate deterministically.
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			out[key] = normalizeYAML(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			out[fmt.Sprint(key)] = normalizeYAML(val)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = normalizeYAML(typed[i])
		}
		return out
	default:
		return typed
	}
}
