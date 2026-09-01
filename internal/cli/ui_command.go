package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/redscaresu/infrafactory/internal/api"
	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/generator"
	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/spf13/cobra"
)

func newUICmd(assets fs.FS) *cobra.Command {
	var addr string
	var allowLayer3 bool

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Serve the InfraFactory web UI",
		RunE: func(cmd *cobra.Command, _ []string) error {
			configPath, err := cmd.Flags().GetString("config")
			if err != nil {
				return formatCommandError("ui", err)
			}

			cfg, err := config.Load(configPath)
			if err != nil {
				return formatCommandError("ui", err)
			}

			// Real-cloud apply is decided HERE, at start time, by the
			// person typing the command in the shell that already holds
			// the credentials -- and nowhere else (ADR-0026, S160b).
			//
			// The config FILE is not allowed to decide it either, which
			// is the stricter half of this rule. `sandbox_deploy.enabled`
			// is a checked-in setting; it says what this repository does
			// when someone runs a scenario deliberately. It should not
			// also mean "and the web server may spend money on its own",
			// because nobody re-reads a config file at the moment they
			// start a UI.
			cfg.Validation.Layers.SandboxDeploy.Enabled = allowLayer3

			hub := api.NewHub()
			go hub.Run(cmd.Context())
			starter := &uiRunStarter{
				configPath: configPath,
				cfg:        cfg,
				hub:        hub,
				baseCtx:    cmd.Context(),
			}

			srv := api.NewServer(api.ServerConfig{
				Addr:       addr,
				Assets:     assets,
				Config:     cfg,
				Store:      runstore.NewFilesystemStore(resolveRunStoreRoot()),
				Hub:        hub,
				RunStarter: starter,
				// Read-only. Deploy, teardown and reap carry guards that
				// live in this package and are not reachable from the API
				// without a seam that does not exist yet (S159a).
				Deployments: livestore.NewFilesystemStore(resolveLiveStoreRoot()),
			})

			errCh := make(chan error, 1)
			go func() {
				if serveErr := srv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					errCh <- serveErr
				}
				close(errCh)
			}()

			select {
			case serveErr, ok := <-errCh:
				if !ok || serveErr == nil {
					return nil
				}
				return formatCommandError("ui", serveErr)
			case <-cmd.Context().Done():
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Shutdown(shutdownCtx); err != nil {
					return formatCommandError("ui", err)
				}
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:4173", "Address to bind UI server")
	cmd.Flags().BoolVar(&allowLayer3, "allow-layer3", false,
		"Permit runs started from this UI to apply to real infrastructure and spend money")

	return cmd
}

type uiRunStarter struct {
	mu             sync.Mutex
	busy           bool
	configPath     string
	cfg            config.Config
	hub            *api.Hub
	baseCtx        context.Context
	resolvedClaude string
	preflightFunc  func() error
	executeRunFunc func(context.Context, string, string) error
}

func (s *uiRunStarter) StartRun(ctx context.Context, req api.StartRunRequest) (string, error) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return "", api.ErrRunBusy
	}
	if err := s.preflight(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	s.busy = true
	s.mu.Unlock()

	runID := time.Now().UTC().Format("20060102T150405Z0700")
	go func() {
		defer func() {
			s.mu.Lock()
			s.busy = false
			s.mu.Unlock()
		}()

		runCtx := context.WithValue(s.runContext(ctx), runIDContextKey{}, runID)
		if err := s.executeRun(runCtx, req, runID); err != nil {
			msg, _ := json.Marshal(map[string]any{"type": "run_error", "data": map[string]any{"error": err.Error()}})
			s.hub.Broadcast(msg)
			return
		}
		msg, _ := json.Marshal(map[string]any{"type": "run_complete", "data": map[string]any{"run_id": runID, "status": "success"}})
		s.hub.Broadcast(msg)
	}()

	return runID, nil
}

func (s *uiRunStarter) preflight() error {
	if s.preflightFunc != nil {
		return s.preflightFunc()
	}
	switch s.cfg.Agent.Type {
	case generator.AgentTypeClaudeCode:
		command := strings.TrimSpace(s.cfg.Agent.Claude.Command)
		if command == "" {
			command = "claude"
		}
		resolved, err := exec.LookPath(command)
		if err != nil {
			return fmt.Errorf("claude CLI unavailable: command %q not found in PATH", command)
		}
		s.resolvedClaude = resolved
	case generator.AgentTypeOpenRouter:
		if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" {
			return fmt.Errorf("openrouter unavailable: OPENROUTER_API_KEY is not set")
		}
	}

	return nil
}

func (s *uiRunStarter) executeRun(ctx context.Context, req api.StartRunRequest, runID string) error {
	if s.executeRunFunc != nil {
		return s.executeRunFunc(ctx, req.ScenarioPath, runID)
	}

	runCmd := &cobra.Command{Use: "run"}
	runCmd.SetOut(io.Discard)
	runCmd.SetErr(io.Discard)
	runCmd.Flags().String("config", config.DefaultPath, "")
	runCmd.Flags().String("output", string(OutputModeJSON), "")
	runCmd.Flags().Int("repair-iterations-max", 0, "")
	runCmd.Flags().Bool("clean", false, "")
	runCmd.Flags().Bool("no-destroy", false, "")
	runCmd.Flags().Bool("reset-mocks", true, "")
	_ = runCmd.Flags().Set("config", s.configPath)
	_ = runCmd.Flags().Set("output", string(OutputModeJSON))
	if req.Clean {
		_ = runCmd.Flags().Set("clean", "true")
	}
	if req.NoDestroy {
		_ = runCmd.Flags().Set("no-destroy", "true")
	}
	runCmd.SetContext(ctx)

	opts := defaultRuntimeOptions()
	opts.configLoader = s.configLoader()

	runtime, err := buildRuntime(runCmd, opts)
	if err != nil {
		return err
	}
	runtime.Logger = NewAppLogger(os.Stderr, api.NewWebSocketSink(s.hub))

	targetPath := req.ScenarioPath
	if filepath.Ext(targetPath) == "" {
		targetPath += ".yaml"
	}
	targetPath = filepath.Join(s.cfg.Paths.Scenarios, filepath.FromSlash(targetPath))
	return runRunCommand(runCmd, []string{targetPath}, runtime)
}

// configLoader builds the per-run configuration for a run this UI server
// starts.
//
// It re-reads the file on every run, which is what makes editing
// `infrafactory.yaml` take effect without a restart. That is also why the
// real-cloud decision has to be re-applied here: a freshly loaded file
// would otherwise bring `sandbox_deploy.enabled: true` back with it and
// quietly re-enable spending on a server the operator started WITHOUT
// `--allow-layer3` (ADR-0026).
//
// Extracted so this is a seam a test can hold, rather than a closure
// buried in the middle of starting a run.
func (s *uiRunStarter) configLoader() func(string) (config.Config, error) {
	resolved := strings.TrimSpace(s.resolvedClaude)
	return func(path string) (config.Config, error) {
		cfg, err := config.Load(path)
		if err != nil {
			return config.Config{}, err
		}
		if s.cfg.Agent.Type == generator.AgentTypeClaudeCode && resolved != "" {
			cfg.Agent.Claude.Command = resolved
		}
		// The server's start-time decision wins over the file, for
		// every run. There is deliberately no request field that can
		// reach this.
		cfg.Validation.Layers.SandboxDeploy.Enabled = s.cfg.Validation.Layers.SandboxDeploy.Enabled
		return cfg, nil
	}
}

func (s *uiRunStarter) runContext(requestCtx context.Context) context.Context {
	if s.baseCtx != nil {
		return s.baseCtx
	}
	if requestCtx != nil {
		return requestCtx
	}
	return context.Background()
}
