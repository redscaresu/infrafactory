package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPath = "./infrafactory.yaml"
)

var (
	ErrInvalidConfig = errors.New("invalid config")
)

type Config struct {
	Version            string            `yaml:"version"`
	Agent              AgentConfig       `yaml:"agent"`
	Mockway            MockwayConfig     `yaml:"mockway"`
	Scaleway           ScalewayConfig    `yaml:"scaleway"`
	Validation         ValidationConfig  `yaml:"validation"`
	ConstraintPolicies map[string]string `yaml:"constraint_policies"`
	Paths              PathsConfig       `yaml:"paths"`
}

type AgentConfig struct {
	Type              string   `yaml:"type"`
	MaxIterations     int      `yaml:"max_iterations"`
	PhaseDelaySeconds int      `yaml:"phase_delay_seconds"`
	Phases            []string `yaml:"phases"`
}

type MockwayConfig struct {
	URL       string `yaml:"url"`
	AutoReset bool   `yaml:"auto_reset"`
}

type ScalewayConfig struct {
	CredentialsSource string `yaml:"credentials_source"`
	SandboxProjectID  string `yaml:"sandbox_project_id"`
}

type ValidationConfig struct {
	Layers ValidationLayers `yaml:"layers"`
}

type ValidationLayers struct {
	Static        StaticLayerConfig `yaml:"static"`
	MockDeploy    LayerConfig       `yaml:"mock_deploy"`
	SandboxDeploy LayerConfig       `yaml:"sandbox_deploy"`
	Destruction   LayerConfig       `yaml:"destruction"`
}

type StaticLayerConfig struct {
	Enabled     bool     `yaml:"enabled"`
	PolicyPaths []string `yaml:"policy_paths"`
}

type LayerConfig struct {
	Enabled bool `yaml:"enabled"`
}

type PathsConfig struct {
	Scenarios string `yaml:"scenarios"`
	Mappings  string `yaml:"mappings"`
	Output    string `yaml:"output"`
	Policies  string `yaml:"policies"`
	Prompts   string `yaml:"prompts"`
}

type FieldError struct {
	Field string
	Err   string
}

type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Fields) == 0 {
		return ErrInvalidConfig.Error()
	}

	message := ErrInvalidConfig.Error()
	for _, fieldErr := range e.Fields {
		message = fmt.Sprintf("%s: %s %s", message, fieldErr.Field, fieldErr.Err)
	}

	return message
}

func (e *ValidationError) Is(target error) bool {
	return target == ErrInvalidConfig
}

func Default() Config {
	return Config{
		Agent: AgentConfig{
			MaxIterations:     5,
			PhaseDelaySeconds: 0,
			Phases: []string{
				"plan_architecture",
				"generate_hcl",
				"self_review",
			},
		},
		Mockway: MockwayConfig{
			AutoReset: true,
		},
		Scaleway: ScalewayConfig{
			CredentialsSource: "env",
			SandboxProjectID:  "",
		},
		Validation: ValidationConfig{
			Layers: ValidationLayers{
				Static: StaticLayerConfig{
					Enabled:     true,
					PolicyPaths: []string{},
				},
				MockDeploy: LayerConfig{
					Enabled: true,
				},
				SandboxDeploy: LayerConfig{
					Enabled: false,
				},
				Destruction: LayerConfig{
					Enabled: true,
				},
			},
		},
		ConstraintPolicies: map[string]string{},
		Paths: PathsConfig{
			Scenarios: "./scenarios",
			Mappings:  "./mappings.yaml",
			Output:    "./output",
			Policies:  "./policies",
			Prompts:   "./prompts",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()

	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config %q: %w", path, err)
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return Config{}, fmt.Errorf("decode config %q: %w", path, err)
		}
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}

	if cfg.ConstraintPolicies == nil {
		cfg.ConstraintPolicies = map[string]string{}
	}
	if cfg.Agent.Phases == nil {
		cfg.Agent.Phases = []string{}
	}
	if cfg.Validation.Layers.Static.PolicyPaths == nil {
		cfg.Validation.Layers.Static.PolicyPaths = []string{}
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validate(cfg Config) error {
	var fields []FieldError

	if cfg.Version == "" {
		fields = append(fields, FieldError{Field: "version", Err: "is required"})
	}

	if cfg.Agent.Type == "" {
		fields = append(fields, FieldError{Field: "agent.type", Err: "is required"})
	}
	if cfg.Agent.Type != "" && cfg.Agent.Type != "claude-code" && cfg.Agent.Type != "openrouter" {
		fields = append(fields, FieldError{Field: "agent.type", Err: "must be one of: claude-code, openrouter"})
	}

	if cfg.Mockway.URL == "" {
		fields = append(fields, FieldError{Field: "mockway.url", Err: "is required"})
	}

	if cfg.Scaleway.CredentialsSource != "env" && cfg.Scaleway.CredentialsSource != "config-file" {
		fields = append(fields, FieldError{Field: "scaleway.credentials_source", Err: "must be one of: env, config-file"})
	}

	if len(fields) == 0 {
		return nil
	}

	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Field < fields[j].Field
	})

	return &ValidationError{Fields: fields}
}
