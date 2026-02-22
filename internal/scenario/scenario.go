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
	Name string `json:"scenario"`
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
