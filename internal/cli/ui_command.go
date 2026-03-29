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
	"github.com/redscaresu/infrafactory/internal/runstore"
	"github.com/spf13/cobra"
)

func newUICmd(assets fs.FS) *cobra.Command {
	var addr string

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
	resolved := strings.TrimSpace(s.resolvedClaude)
	opts.configLoader = func(path string) (config.Config, error) {
		cfg, err := config.Load(path)
		if err != nil {
			return config.Config{}, err
		}
		if s.cfg.Agent.Type == generator.AgentTypeClaudeCode && resolved != "" {
			cfg.Agent.Claude.Command = resolved
		}
		if req.Layer3Enabled != nil {
			cfg.Validation.Layers.SandboxDeploy.Enabled = *req.Layer3Enabled
		}
		return cfg, nil
	}

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

func (s *uiRunStarter) runContext(requestCtx context.Context) context.Context {
	if s.baseCtx != nil {
		return s.baseCtx
	}
	if requestCtx != nil {
		return requestCtx
	}
	return context.Background()
}


