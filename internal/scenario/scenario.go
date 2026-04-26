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
	Name               string                `json:"scenario"`
	Version            string                `json:"version"`
	Cloud              string                `json:"cloud"`
	Description        string                `json:"description"`
	Type               string                `json:"type,omitempty"`
	References         string                `json:"references,omitempty"`
	Resources          Resources             `json:"resources"`
	Constraints        map[string]any        `json:"constraints,omitempty"`
	AcceptanceCriteria []AcceptanceCriterion `json:"acceptance_criteria"`
}

type Resources struct {
	Compute    *ComputeResource    `json:"compute,omitempty"`
	Networking *NetworkingResource `json:"networking,omitempty"`
	Database   *DatabaseResource   `json:"database,omitempty"`
	Kubernetes *KubernetesResource `json:"kubernetes,omitempty"`
	IAM        *IAMResource        `json:"iam,omitempty"`
	Registry   *RegistryResource   `json:"registry,omitempty"`
	Redis      *RedisResource      `json:"redis,omitempty"`
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
	Exposure string      `json:"exposure"`
	Backends []LBBackend `json:"backends"`
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

type RedisResource struct {
	Purpose  string        `json:"purpose"`
	Size     string        `json:"size"`
	Override RedisOverride `json:"override,omitempty"`
}

type RedisOverride struct {
	NodeType string `json:"node_type,omitempty"`
}

type RegistryResource struct {
	Purpose  string `json:"purpose"`
	IsPublic bool   `json:"is_public,omitempty"`
}

type IAMResource struct {
	Purpose     string `json:"purpose"`
	Application bool   `json:"application,omitempty"`
	APIKey      bool   `json:"api_key,omitempty"`
	Policy      bool   `json:"policy,omitempty"`
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
	Path    string `json:"path"`
	Message string `json:"message"`
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
	return parseAndValidate(payload, path, schemaPath)
}

// ValidateBytes validates raw scenario YAML bytes against the JSON schema
// without writing to disk. It returns nil on success, ErrMalformedScenario
// (wrapped) for YAML syntax errors, or *ValidationError for schema
// violations. The sourceLabel is included in error messages for context
// (callers may pass an empty string when there's no path, e.g. an editor
// buffer).
func ValidateBytes(payload []byte, schemaPath, sourceLabel string) error {
	_, err := parseAndValidate(payload, sourceLabel, schemaPath)
	return err
}

func parseAndValidate(payload []byte, sourceLabel, schemaPath string) (Scenario, error) {
	var raw any
	if err := yaml.Unmarshal(payload, &raw); err != nil {
		return Scenario{}, fmt.Errorf("%w: parse scenario %q: %v", ErrMalformedScenario, sourceLabel, err)
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
				ScenarioPath: sourceLabel,
				Violations:   violations,
			}
		}
		return Scenario{}, fmt.Errorf("%w: validate scenario %q: %v", ErrInvalidScenario, sourceLabel, err)
	}

	jsonBytes, err := json.Marshal(normalized)
	if err != nil {
		return Scenario{}, fmt.Errorf("marshal scenario %q: %w", sourceLabel, err)
	}

	var result Scenario
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return Scenario{}, fmt.Errorf("decode scenario %q: %w", sourceLabel, err)
	}

	applyIAMDefaults(&result, normalized)

	return result, nil
}

// applyIAMDefaults sets IAM boolean fields to their schema defaults (true)
// when the user omitted them. Go's json.Unmarshal decodes missing bools as
// false, but the JSON Schema declares default: true for application, api_key,
// and policy.
func applyIAMDefaults(sc *Scenario, normalized any) {
	if sc.Resources.IAM == nil {
		return
	}
	root, ok := normalized.(map[string]any)
	if !ok {
		return
	}
	resources, ok := root["resources"].(map[string]any)
	if !ok {
		return
	}
	iam, ok := resources["iam"].(map[string]any)
	if !ok {
		return
	}
	if _, present := iam["application"]; !present {
		sc.Resources.IAM.Application = true
	}
	if _, present := iam["api_key"]; !present {
		sc.Resources.IAM.APIKey = true
	}
	if _, present := iam["policy"]; !present {
		sc.Resources.IAM.Policy = true
	}
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
