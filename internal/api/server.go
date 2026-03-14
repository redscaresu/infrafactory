package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/runstore"
)

const uiAssetsMissingMessage = "UI assets not embedded. Run Vite dev server on :5173 or build with: make build"

type ServerConfig struct {
	Addr          string
	Assets        fs.FS
	Config        config.Config
	Store         *runstore.FilesystemStore
	Formatter     IaCFormatter
	Hub           *Hub
	SchemaPath    string
	RunStarter    RunStarter
	RuntimeErrors chan error
}

type RunStarter interface {
	StartRun(ctx context.Context, scenarioName, scenarioPath string) (runID string, err error)
}

type IaCFormatter interface {
	Format(ctx context.Context, filename string, content []byte) ([]byte, error)
}

func NewServer(cfg ServerConfig) *http.Server {
	store := cfg.Store
	if store == nil {
		store = runstore.NewFilesystemStore(runstore.DefaultRoot)
	}

	state := &serverState{
		cfg:        cfg.Config,
		store:      store,
		formatter:  cfg.Formatter,
		hub:        cfg.Hub,
		schemaPath: cfg.SchemaPath,
		runStarter: cfg.RunStarter,
		sessionID:  fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UTC().UnixNano()),
		startedAt:  time.Now().UTC(),
	}
	if state.hub == nil {
		state.hub = NewHub()
	}
	if state.formatter == nil {
		state.formatter = NewExternalIaCFormatter()
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/config", newConfigHandler(state))
	mux.HandleFunc("/api/diagnostics", diagnosticsHandler(state))
	mux.HandleFunc("/api/scenarios", listScenariosHandler(state))
	mux.HandleFunc("/api/scenarios/", scenarioByPathHandler(state))
	mux.HandleFunc("/api/runs", listAllRunsHandler(state))
	mux.HandleFunc("/api/runs/", runsByScenarioHandler(state))
	mux.HandleFunc("/api/output/", outputHandler(state))
	mux.HandleFunc("/api/ws", websocketHandler(state))

	mux.HandleFunc("/api", notImplementedAPIHandler)
	mux.HandleFunc("/api/", notImplementedAPIHandler)

	if cfg.Assets != nil {
		mux.Handle("/", SPAHandler(cfg.Assets))
	} else {
		mux.HandleFunc("/", uiAssetsMissingHandler)
	}

	return &http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
	}
}

type serverState struct {
	cfg        config.Config
	store      *runstore.FilesystemStore
	formatter  IaCFormatter
	hub        *Hub
	schemaPath string
	runStarter RunStarter
	sessionID string
	startedAt time.Time
}

func (s *serverState) scenarioSchemaPathCandidates() []string {
	if strings.TrimSpace(s.schemaPath) != "" {
		return []string{s.schemaPath}
	}
	return []string{
		"scenario.schema.json",
		filepath.Join("..", "..", "scenario.schema.json"),
	}
}

func notImplementedAPIHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "not implemented",
	})
}

func uiAssetsMissingHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotFound, map[string]string{
		"error": uiAssetsMissingMessage,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func websocketNotConfiguredHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSONError(w, http.StatusNotImplemented, "websocket not configured")
}

func websocketHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{
				"127.0.0.1",
				"127.0.0.1:*",
				"localhost",
				"localhost:*",
				"http://127.0.0.1",
				"http://127.0.0.1:*",
				"https://127.0.0.1",
				"https://127.0.0.1:*",
				"http://localhost",
				"http://localhost:*",
				"https://localhost",
				"https://localhost:*",
			},
		})
		if err != nil {
			return
		}

		client := newClient(state.hub, conn)
		state.hub.Register(client)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			client.WritePump(ctx)
			cancel()
		}()
		client.ReadPump(ctx)
	}
}
