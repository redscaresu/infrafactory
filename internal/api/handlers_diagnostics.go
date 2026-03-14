package api

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/generator"
)

type diagnosticsResponse struct {
	AgentType    string            `json:"agent_type"`
	Ready        bool              `json:"ready"`
	Summary      string            `json:"summary"`
	Checks       []diagnosticCheck `json:"checks"`
	SessionID    string            `json:"session_id,omitempty"`
	StartedAt    string            `json:"started_at,omitempty"`
	Limitations  []string          `json:"limitations,omitempty"`
}

type diagnosticCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func diagnosticsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		resp := buildDiagnosticsResponse(state)
		writeJSON(w, http.StatusOK, resp)
	}
}

func buildDiagnosticsResponse(state *serverState) diagnosticsResponse {
	sessionID := state.sessionID
	if strings.TrimSpace(sessionID) == "" {
		sessionID = "unknown"
	}
	startedAt := ""
	if !state.startedAt.IsZero() {
		startedAt = state.startedAt.Format(time.RFC3339)
	}

	resp := diagnosticsResponse{
		AgentType: state.cfg.Agent.Type,
		Ready:     true,
		Summary:   "Generator runtime looks available.",
		Checks:    make([]diagnosticCheck, 0, 2),
		SessionID: sessionID,
		StartedAt: startedAt,
		Limitations: []string{
			"These checks confirm local runtime prerequisites only.",
			"They do not guarantee provider auth, model auth, or a successful generation result.",
		},
	}

	switch state.cfg.Agent.Type {
	case generator.AgentTypeClaudeCode:
		command := strings.TrimSpace(state.cfg.Agent.Claude.Command)
		if command == "" {
			command = "claude"
		}

		path, err := exec.LookPath(command)
		if err != nil {
			resp.Ready = false
			resp.Summary = "Claude CLI is unavailable."
			resp.Checks = append(resp.Checks, diagnosticCheck{
				Name:   "claude_command",
				Status: "fail",
				Detail: `command "` + command + `" not found in PATH`,
			})
			return resp
		}

		resp.Checks = append(resp.Checks, diagnosticCheck{
			Name:   "claude_command",
			Status: "pass",
			Detail: "found at " + path,
		})
	case generator.AgentTypeOpenRouter:
		if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" {
			resp.Ready = false
			resp.Summary = "OpenRouter API key is unavailable."
			resp.Checks = append(resp.Checks, diagnosticCheck{
				Name:   "openrouter_api_key",
				Status: "fail",
				Detail: "OPENROUTER_API_KEY is not set",
			})
			return resp
		}

		resp.Checks = append(resp.Checks, diagnosticCheck{
			Name:   "openrouter_api_key",
			Status: "pass",
			Detail: "OPENROUTER_API_KEY is set",
		})
	default:
		resp.Ready = false
		resp.Summary = "Generator type is unsupported."
		resp.Checks = append(resp.Checks, diagnosticCheck{
			Name:   "agent_type",
			Status: "fail",
			Detail: `unsupported agent type "` + state.cfg.Agent.Type + `"`,
		})
	}

	return resp
}
