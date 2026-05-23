package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	MockState     MockStateReader
	FakegcpState  MockStateReader
	Formatter     IaCFormatter
	Hub           *Hub
	SchemaPath    string
	RunStarter    RunStarter
	RuntimeErrors chan error
}

type StartRunRequest struct {
	ScenarioName  string `json:"-"`
	ScenarioPath  string `json:"-"`
	Clean         bool   `json:"clean"`
	NoDestroy     bool   `json:"no_destroy"`
	Layer3Enabled *bool  `json:"layer3_enabled,omitempty"`
}

type RunStarter interface {
	StartRun(ctx context.Context, req StartRunRequest) (runID string, err error)
}

type MockStateReader interface {
	State(ctx context.Context) ([]byte, error)
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
		cfg:          cfg.Config,
		store:        store,
		mockState:    cfg.MockState,
		fakegcpState: cfg.FakegcpState,
		formatter:    cfg.Formatter,
		hub:          cfg.Hub,
		schemaPath:   cfg.SchemaPath,
		runStarter:   cfg.RunStarter,
		sessionID:    fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UTC().UnixNano()),
		startedAt:    time.Now().UTC(),
	}
	if state.hub == nil {
		state.hub = NewHub()
	}
	if state.mockState == nil && strings.TrimSpace(cfg.Config.Mockway.URL) != "" {
		state.mockState = &httpMockStateClient{
			baseURL: strings.TrimRight(cfg.Config.Mockway.URL, "/"),
			client:  &http.Client{Timeout: 5 * time.Second},
		}
	}
	if state.fakegcpState == nil && strings.TrimSpace(cfg.Config.Fakegcp.URL) != "" {
		state.fakegcpState = &httpMockStateClient{
			baseURL: strings.TrimRight(cfg.Config.Fakegcp.URL, "/"),
			client:  &http.Client{Timeout: 5 * time.Second},
		}
	}
	if state.formatter == nil {
		state.formatter = NewExternalIaCFormatter()
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/config", newConfigHandler(state))
	mux.HandleFunc("/api/diagnostics", diagnosticsHandler(state))
	mux.HandleFunc("/api/scenarios", listScenariosHandler(state))
	mux.HandleFunc("/api/scenarios/validate", validateScenarioHandler(state))
	mux.HandleFunc("/api/scenarios/", scenarioByPathHandler(state))
	mux.HandleFunc("/api/runs", listAllRunsHandler(state))
	mux.HandleFunc("/api/runs/", runsByScenarioHandler(state))
	mux.HandleFunc("/api/output/", outputHandler(state))
	mux.HandleFunc("/api/pitfalls", pitfallsHandler(state))
	mux.HandleFunc("/api/pitfalls/", pitfallsHandler(state))
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
	cfg          config.Config
	store        *runstore.FilesystemStore
	mockState    MockStateReader
	fakegcpState MockStateReader
	formatter    IaCFormatter
	hub          *Hub
	schemaPath   string
	runStarter   RunStarter
	sessionID    string
	startedAt    time.Time
}

// mockStateForCloud picks the mock-state reader appropriate for the
// scenario's cloud field. Falls back to mockState (Scaleway) for empty/
// unknown clouds to preserve pre-multi-cloud behavior.
func (s *serverState) mockStateForCloud(cloud string) (MockStateReader, string) {
	switch strings.ToLower(strings.TrimSpace(cloud)) {
	case "gcp":
		if s.fakegcpState != nil {
			return s.fakegcpState, "fakegcp"
		}
	}
	return s.mockState, "mockway"
}

type httpMockStateClient struct {
	baseURL string
	client  *http.Client
}

func (c *httpMockStateClient) State(ctx context.Context) ([]byte, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("mock state client is not configured")
	}
	client := c.client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/mock/state", nil)
	if err != nil {
		return nil, fmt.Errorf("build mock state request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch mock state: %w", err)
	}
	defer resp.Body.Close()

	const maxMockStateBytes = 8 << 20 // 8 MB, consistent with CLI limit
	payload, readErr := io.ReadAll(io.LimitReader(resp.Body, maxMockStateBytes+1))
	if readErr != nil {
		return nil, fmt.Errorf("read mock state response: %w", readErr)
	}
	// Status check first so a non-2xx surfaces with the upstream's
	// (truncated) body rather than as "payload exceeds 8 MB". Tolerance
	// matches the CLI sibling: any 2xx is acceptable.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := strings.TrimSpace(string(payload))
		const maxErrPayload = 1024
		if len(body) > maxErrPayload {
			body = body[:maxErrPayload] + "..."
		}
		return nil, fmt.Errorf("fetch mock state: unexpected status %d: %s", resp.StatusCode, body)
	}
	if len(payload) > maxMockStateBytes {
		return nil, fmt.Errorf("read mock state response: payload exceeds %d bytes", maxMockStateBytes)
	}
	return payload, nil
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
