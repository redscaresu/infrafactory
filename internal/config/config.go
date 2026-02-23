package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/redscaresu/infrafactory/internal/generator"
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
	Type                string           `yaml:"type"`
	RepairIterationsMax int              `yaml:"repair_iterations_max"`
	IterationsTarget    int              `yaml:"iterations_target"`
	PhaseDelaySeconds   int              `yaml:"phase_delay_seconds"`
	Phases              []string         `yaml:"phases"`
	Claude              ClaudeConfig     `yaml:"claude"`
	OpenRouter          OpenRouterConfig `yaml:"openrouter"`
}

type ClaudeConfig struct {
	Command             string `yaml:"command"`
	PhaseTimeoutSeconds int    `yaml:"phase_timeout_seconds"`
}

type OpenRouterConfig struct {
	Model          string `yaml:"model"`
	BaseURL        string `yaml:"base_url"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	MaxRetries     int    `yaml:"max_retries"`
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
			RepairIterationsMax: 5,
			IterationsTarget:    1,
			PhaseDelaySeconds:   0,
			Phases: []string{
				generator.PhasePlanArchitecture,
				generator.PhaseGenerateHCL,
				generator.PhaseSelfReview,
			},
			Claude: ClaudeConfig{
				Command:             "claude",
				PhaseTimeoutSeconds: 300,
			},
			OpenRouter: OpenRouterConfig{
				BaseURL:        "https://openrouter.ai/api/v1",
				TimeoutSeconds: 60,
				MaxRetries:     2,
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
			return Config{}, fmt.Errorf("decode config %q: empty config file", path)
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
	if cfg.Agent.RepairIterationsMax < 1 {
		fields = append(fields, FieldError{Field: "agent.repair_iterations_max", Err: "must be greater than or equal to 1"})
	}
	if cfg.Agent.IterationsTarget < 1 {
		fields = append(fields, FieldError{Field: "agent.iterations_target", Err: "must be greater than or equal to 1"})
	}
	if cfg.Agent.Type != "" && cfg.Agent.Type != generator.AgentTypeClaudeCode && cfg.Agent.Type != generator.AgentTypeOpenRouter {
		fields = append(fields, FieldError{Field: "agent.type", Err: "must be one of: claude-code, openrouter"})
	}
	if cfg.Agent.PhaseDelaySeconds < 0 {
		fields = append(fields, FieldError{Field: "agent.phase_delay_seconds", Err: "must be greater than or equal to 0"})
	}
	if len(cfg.Agent.Phases) == 0 {
		fields = append(fields, FieldError{Field: "agent.phases", Err: "must include at least one phase"})
	} else {
		seen := map[string]struct{}{}
		validPhases := map[string]struct{}{}
		for _, phase := range generator.SupportedPhases {
			validPhases[phase] = struct{}{}
		}

		for i, phase := range cfg.Agent.Phases {
			field := fmt.Sprintf("agent.phases[%d]", i)
			if phase == "" {
				fields = append(fields, FieldError{Field: field, Err: "must not be empty"})
				continue
			}
			if _, ok := validPhases[phase]; !ok {
				fields = append(fields, FieldError{Field: field, Err: "is not supported"})
			}
			if _, exists := seen[phase]; exists {
				fields = append(fields, FieldError{Field: field, Err: "must not contain duplicates"})
			}
			seen[phase] = struct{}{}
		}

		if len(cfg.Agent.Phases) != len(generator.SupportedPhases) {
			fields = append(fields, FieldError{
				Field: "agent.phases",
				Err:   fmt.Sprintf("must contain exactly: %s", strings.Join(generator.SupportedPhases, ", ")),
			})
		} else {
			for i := range generator.SupportedPhases {
				if cfg.Agent.Phases[i] != generator.SupportedPhases[i] {
					fields = append(fields, FieldError{
						Field: "agent.phases",
						Err:   fmt.Sprintf("must preserve phase order: %s", strings.Join(generator.SupportedPhases, ", ")),
					})
					break
				}
			}
		}
	}

	switch cfg.Agent.Type {
	case generator.AgentTypeClaudeCode:
		if cfg.Agent.Claude.Command == "" {
			fields = append(fields, FieldError{Field: "agent.claude.command", Err: "is required when agent.type=claude-code"})
		}
		if cfg.Agent.Claude.PhaseTimeoutSeconds <= 0 {
			fields = append(fields, FieldError{Field: "agent.claude.phase_timeout_seconds", Err: "must be greater than 0"})
		}
	case generator.AgentTypeOpenRouter:
		if cfg.Agent.OpenRouter.Model == "" {
			fields = append(fields, FieldError{Field: "agent.openrouter.model", Err: "is required when agent.type=openrouter"})
		}
		if cfg.Agent.OpenRouter.BaseURL == "" {
			fields = append(fields, FieldError{Field: "agent.openrouter.base_url", Err: "is required when agent.type=openrouter"})
		}
		if cfg.Agent.OpenRouter.TimeoutSeconds <= 0 {
			fields = append(fields, FieldError{Field: "agent.openrouter.timeout_seconds", Err: "must be greater than 0"})
		}
		if cfg.Agent.OpenRouter.MaxRetries < 0 {
			fields = append(fields, FieldError{Field: "agent.openrouter.max_retries", Err: "must be greater than or equal to 0"})
		}
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
