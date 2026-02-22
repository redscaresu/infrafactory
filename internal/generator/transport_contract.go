package generator

import (
	"errors"
	"fmt"
)

const (
	AgentTypeClaudeCode = "claude-code"
	AgentTypeOpenRouter = "openrouter"

	PhasePlanArchitecture = "plan_architecture"
	PhaseGenerateHCL      = "generate_hcl"
	PhaseSelfReview       = "self_review"
)

var (
	ErrUnknownTransport = errors.New("unknown generator transport")
)

var SupportedPhases = []string{
	PhasePlanArchitecture,
	PhaseGenerateHCL,
	PhaseSelfReview,
}

type TransportContract struct {
	AgentType           string
	RequiredEnv         []string
	RequiredConfigPaths []string
}

func ContractForAgentType(agentType string) (TransportContract, error) {
	switch agentType {
	case AgentTypeClaudeCode:
		return TransportContract{
			AgentType:   AgentTypeClaudeCode,
			RequiredEnv: []string{},
			RequiredConfigPaths: []string{
				"agent.type",
				"agent.claude.command",
				"agent.phases",
				"agent.phase_delay_seconds",
			},
		}, nil
	case AgentTypeOpenRouter:
		return TransportContract{
			AgentType: AgentTypeOpenRouter,
			RequiredEnv: []string{
				"OPENROUTER_API_KEY",
			},
			RequiredConfigPaths: []string{
				"agent.type",
				"agent.openrouter.model",
				"agent.openrouter.base_url",
				"agent.openrouter.timeout_seconds",
				"agent.openrouter.max_retries",
				"agent.phases",
				"agent.phase_delay_seconds",
			},
		}, nil
	default:
		return TransportContract{}, fmt.Errorf("%w: %q", ErrUnknownTransport, agentType)
	}
}
