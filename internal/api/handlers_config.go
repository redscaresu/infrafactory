package api

import (
	"net/http"

	"github.com/redscaresu/infrafactory/internal/config"
)

type configResponse struct {
	Version    string                  `json:"version"`
	Agent      configResponseAgent     `json:"agent"`
	Paths      configResponsePaths     `json:"paths"`
	Validation config.ValidationConfig `json:"validation"`
}

type configResponseAgent struct {
	Type                string                        `json:"type"`
	RepairIterationsMax int                           `json:"repair_iterations_max"`
	PhaseDelaySeconds   int                           `json:"phase_delay_seconds"`
	Phases              []string                      `json:"phases"`
	OpenRouter          configResponseAgentOpenRouter `json:"openrouter"`
}

type configResponseAgentOpenRouter struct {
	Model   string `json:"model"`
	BaseURL string `json:"base_url"`
}

type configResponsePaths struct {
	Scenarios string `json:"scenarios"`
	Output    string `json:"output"`
}

func newConfigHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg := state.cfg
		resp := configResponse{
			Version: cfg.Version,
			Agent: configResponseAgent{
				Type:                cfg.Agent.Type,
				RepairIterationsMax: cfg.Agent.RepairIterationsMax,
				PhaseDelaySeconds:   cfg.Agent.PhaseDelaySeconds,
				Phases:              append([]string(nil), cfg.Agent.Phases...),
				OpenRouter: configResponseAgentOpenRouter{
					Model:   cfg.Agent.OpenRouter.Model,
					BaseURL: cfg.Agent.OpenRouter.BaseURL,
				},
			},
			Paths: configResponsePaths{
				Scenarios: cfg.Paths.Scenarios,
				Output:    cfg.Paths.Output,
			},
			Validation: cfg.Validation,
		}
		writeJSON(w, http.StatusOK, resp)
	}
}
