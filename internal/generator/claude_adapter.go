package generator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ClaudeTransportConfig struct {
	Command          string
	PromptsDir       string
	Phases           []string
	PhaseDelay       time.Duration
	PhaseTimeout     time.Duration
	ProgressWriter   io.Writer
	Environment      map[string]string
	Constraints      string
	ResolvedMappings string
	Overrides        string
	Acceptance       string
}

type ClaudeCommandRequest struct {
	Command string
	Args    []string
	Env     map[string]string
}

type ClaudeCommandRunner interface {
	Run(context.Context, ClaudeCommandRequest) ([]byte, error)
}

type ClaudeSeedGenerator struct {
	cfg    ClaudeTransportConfig
	runner ClaudeCommandRunner
	sleep  func(time.Duration)
}

func NewClaudeSeedGenerator(cfg ClaudeTransportConfig, runner ClaudeCommandRunner) (*ClaudeSeedGenerator, error) {
	if cfg.Command == "" {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("claude command is required"))
	}
	if cfg.PromptsDir == "" {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("prompts dir is required"))
	}
	if len(cfg.Phases) == 0 {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("at least one phase is required"))
	}
	if cfg.PhaseDelay < 0 {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("phase delay must be >= 0"))
	}
	if cfg.PhaseTimeout < 0 {
		return nil, NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("phase timeout must be >= 0"))
	}
	for _, phase := range cfg.Phases {
		if _, err := phaseTemplateFile(phase); err != nil {
			return nil, err
		}
	}
	if runner == nil {
		runner = claudeExecRunner{}
	}

	return &ClaudeSeedGenerator{
		cfg:    cfg,
		runner: runner,
		sleep:  time.Sleep,
	}, nil
}

func (g *ClaudeSeedGenerator) Generate(ctx context.Context, req Request) (*GeneratedCode, error) {
	if len(req.ScenarioYAML) == 0 {
		return nil, NewGenerateError(ErrGenerateFailed, "request", fmt.Errorf("scenario YAML is required"))
	}

	phaseResults := make([]PhaseResult, 0, len(g.cfg.Phases))
	phaseOutput := map[string]string{}
	lastFiles := map[string][]byte{}

	for i, phase := range g.cfg.Phases {
		g.logProgress("phase %q start\n", phase)

		prompt, err := g.renderPhasePrompt(phase, req, phaseOutput, lastFiles)
		if err != nil {
			return nil, err
		}

		phaseCtx := ctx
		cancel := func() {}
		if g.cfg.PhaseTimeout > 0 {
			phaseCtx, cancel = context.WithTimeout(ctx, g.cfg.PhaseTimeout)
		}
		out, err := g.runner.Run(phaseCtx, ClaudeCommandRequest{
			Command: g.cfg.Command,
			Args:    []string{"-p", prompt},
			Env:     g.cfg.Environment,
		})
		cancel()
		if err != nil {
			detail := redactTransportDetail(err.Error(), prompt, g.cfg.Environment)
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(phaseCtx.Err(), context.DeadlineExceeded) {
				detail = fmt.Sprintf("phase timed out after %s: %s", g.cfg.PhaseTimeout, detail)
			}
			g.logProgress("phase %q failed\n", phase)
			return nil, NewGenerateError(ErrTransportFailed, phase, fmt.Errorf("run %q: %s", g.cfg.Command, detail))
		}
		phaseText := string(out)
		phaseOutput[phase] = phaseText
		phaseResults = append(phaseResults, PhaseResult{
			Name:   phase,
			Prompt: []byte(prompt),
			Output: out,
		})
		g.logProgress("phase %q complete\n", phase)

		switch phase {
		case PhaseGenerateHCL:
			files, parseErr := ParseFileBlocks(phaseText)
			if parseErr != nil {
				return nil, NewGenerateError(ErrParseFailed, phase, parseErr)
			}
			lastFiles = files
		case PhaseSelfReview:
			trimmed := strings.TrimSpace(phaseText)
			if strings.EqualFold(trimmed, "NO ISSUES FOUND") {
				break
			}
			files, parseErr := ParseFileBlocks(phaseText)
			if parseErr != nil {
				return nil, NewGenerateError(ErrParseFailed, phase, parseErr)
			}
			if lastFiles == nil {
				lastFiles = make(map[string][]byte, len(files))
			}
			for name, content := range files {
				lastFiles[name] = content
			}
		}

		if i < len(g.cfg.Phases)-1 && g.cfg.PhaseDelay > 0 {
			g.sleep(g.cfg.PhaseDelay)
		}
	}

	result := &GeneratedCode{
		Files: lastFiles,
		Metadata: GenerationMetadata{
			Generator: AgentTypeClaudeCode,
			Phases:    phaseResults,
		},
	}
	if err := result.Validate(); err != nil {
		return nil, NewGenerateError(ErrParseFailed, "finalize", err)
	}

	return result, nil
}

func (g *ClaudeSeedGenerator) logProgress(format string, args ...any) {
	if g.cfg.ProgressWriter == nil {
		return
	}
	_, _ = fmt.Fprintf(g.cfg.ProgressWriter, "generator/claude: "+format, args...)
}

func (g *ClaudeSeedGenerator) renderPhasePrompt(phase string, req Request, outputs map[string]string, files map[string][]byte) (string, error) {
	fileName, err := phaseTemplateFile(phase)
	if err != nil {
		return "", err
	}
	templatePath := filepath.Join(g.cfg.PromptsDir, fileName)
	ctx := PromptContext{
		ScenarioYAML:       string(req.ScenarioYAML),
		Constraints:        g.cfg.Constraints,
		ResolvedMappings:   g.cfg.ResolvedMappings,
		Overrides:          g.cfg.Overrides,
		ArchitecturePlan:   outputs[PhasePlanArchitecture],
		AcceptanceCriteria: g.cfg.Acceptance,
		GeneratedFiles:     renderGeneratedFiles(files),
		FeedbackJSON:       string(req.FeedbackJSON),
	}

	return RenderPromptFile(phase, templatePath, ctx)
}

func phaseTemplateFile(phase string) (string, error) {
	switch phase {
	case PhasePlanArchitecture:
		return "phase1_plan_architecture.md", nil
	case PhaseGenerateHCL:
		return "phase2_generate_hcl.md", nil
	case PhaseSelfReview:
		return "phase3_self_review.md", nil
	default:
		return "", NewGenerateError(ErrTransportFailed, "config", fmt.Errorf("unsupported phase %q", phase))
	}
}

func renderGeneratedFiles(files map[string][]byte) string {
	if len(files) == 0 {
		return ""
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var out bytes.Buffer
	for i, name := range names {
		_, _ = fmt.Fprintf(&out, "# File: %s\n```hcl\n%s\n```\n", name, string(files[name]))
		if i < len(names)-1 {
			_, _ = fmt.Fprintln(&out)
		}
	}

	return out.String()
}

type claudeExecRunner struct{}

func (claudeExecRunner) Run(ctx context.Context, req ClaudeCommandRequest) ([]byte, error) {
	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	if len(req.Env) > 0 {
		envPairs := make([]string, 0, len(req.Env))
		for k, v := range req.Env {
			envPairs = append(envPairs, fmt.Sprintf("%s=%s", k, v))
		}
		sort.Strings(envPairs)
		cmd.Env = append(cmd.Environ(), envPairs...)
	}

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return out, nil
}
