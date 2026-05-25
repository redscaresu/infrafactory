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
	Fakegcp            FakegcpConfig     `yaml:"fakegcp"`
	Fakeaws            FakeawsConfig     `yaml:"fakeaws"`
	S3                 S3Config          `yaml:"s3"`
	Scaleway           ScalewayConfig    `yaml:"scaleway"`
	Validation         ValidationConfig  `yaml:"validation"`
	ConstraintPolicies map[string]string `yaml:"constraint_policies"`
	Paths              PathsConfig       `yaml:"paths"`
}

type AgentConfig struct {
	Type                string           `yaml:"type"`
	RepairIterationsMax int              `yaml:"repair_iterations_max"`
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

// FakegcpConfig points the runtime at the GCP mock when a scenario
// declares `cloud: gcp`. Optional — if URL is empty, GCP scenarios
// fall back to Mockway.URL (which would 4xx on GCP-shaped HCL but
// keeps the runtime constructible).
type FakegcpConfig struct {
	URL       string `yaml:"url"`
	AutoReset bool   `yaml:"auto_reset"`
}

// FakeawsConfig points the runtime at the AWS mock when a scenario
// declares `cloud: aws`. Same shape as FakegcpConfig: URL is the
// HTTP endpoint where fakeaws is listening (default :8082 — mockway
// owns :8080, fakegcp :8081). Optional — if URL is empty, AWS
// scenarios fall back to Mockway.URL (which would 4xx on AWS HCL
// but keeps the runtime constructible). Added in S43-T9 per
// fakeaws/concepts.md "Required surface" item 4.
type FakeawsConfig struct {
	URL       string `yaml:"url"`
	AutoReset bool   `yaml:"auto_reset"`
}

// S3Config points the runtime at the third-party S3 backend used in
// place of fakeaws's built-in S3 handler (decision documented in
// CONCEPT.md "Third-Party Mock Integration" section, M59). Default
// backend is SeaweedFS — Apache 2.0, full S3 surface, ~50 MB Go
// binary. When URL is non-empty, the cloudMockStateRouter dispatches
// AWS S3 calls here while everything else stays on Fakeaws.URL.
// Optional — if URL is empty, S3 calls fall back to fakeaws's
// stripped-down S3 surface (kept for direct-HTTP tests, not viable
// for terraform-provider-aws Read flows).
type S3Config struct {
	URL       string `yaml:"url"`
	AutoReset bool   `yaml:"auto_reset"`
}

type ScalewayConfig struct {
	CredentialsSource string `yaml:"credentials_source"`
}

type ValidationConfig struct {
	Layers     ValidationLayers `yaml:"layers"`
	RealProbes RealProbeConfig  `yaml:"real_probes"`
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

type RealProbeConfig struct {
	TimeoutSeconds    int `yaml:"timeout_seconds"`
	Retries           int `yaml:"retries"`
	RetryDelaySeconds int `yaml:"retry_delay_seconds"`
}

type PathsConfig struct {
	Scenarios string `yaml:"scenarios"`
	Mappings  string `yaml:"mappings"`
	Output    string `yaml:"output"`
	Policies  string `yaml:"policies"`
	Prompts   string `yaml:"prompts"`
	Pitfalls  string `yaml:"pitfalls"`
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
		Fakegcp: FakegcpConfig{
			AutoReset: true,
		},
		S3: S3Config{
			AutoReset: true,
		},
		Scaleway: ScalewayConfig{
			CredentialsSource: "env",
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
			RealProbes: RealProbeConfig{
				TimeoutSeconds:    5,
				Retries:           6,
				RetryDelaySeconds: 5,
			},
		},
		ConstraintPolicies: map[string]string{},
		Paths: PathsConfig{
			Scenarios: "./scenarios",
			Mappings:  "./mappings.yaml",
			Output:    "./output",
			Policies:  "./policies",
			Prompts:   "./prompts",
			Pitfalls:  "./pitfalls",
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
	if cfg.Validation.RealProbes.TimeoutSeconds < 1 {
		fields = append(fields, FieldError{Field: "validation.real_probes.timeout_seconds", Err: "must be greater than or equal to 1"})
	}
	if cfg.Validation.RealProbes.Retries < 1 {
		fields = append(fields, FieldError{Field: "validation.real_probes.retries", Err: "must be greater than or equal to 1"})
	}
	if cfg.Validation.RealProbes.RetryDelaySeconds < 0 {
		fields = append(fields, FieldError{Field: "validation.real_probes.retry_delay_seconds", Err: "must be greater than or equal to 0"})
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
