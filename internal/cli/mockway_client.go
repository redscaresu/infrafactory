package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxMockwayStateResponseBytes = 8 * 1024 * 1024
const maxMockwayErrorPayloadBytes = 1024

type mockStateClient struct {
	baseURL string
	client  *http.Client
}

// newMockStateClient builds an HTTP client for the
// `/mock/{state,reset,snapshot,restore}` admin endpoints. mockway and
// fakegcp expose the same endpoint shapes, so the same client wires up
// either backend — but the runtime currently passes
// cfg.Mockway.URL unconditionally; selecting the URL by scenario cloud
// is a follow-up (tracked outside Slice 36).
func newMockStateClient(baseURL string) *mockStateClient {
	return &mockStateClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *mockStateClient) Reset(ctx context.Context) error {
	return c.postNoBody(ctx, "/mock/reset", "reset mock state")
}

func (c *mockStateClient) Snapshot(ctx context.Context) error {
	return c.postNoBody(ctx, "/mock/snapshot", "snapshot mock state")
}

func (c *mockStateClient) Restore(ctx context.Context) error {
	return c.postNoBody(ctx, "/mock/restore", "restore mock state")
}

func (c *mockStateClient) State(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/mock/state", nil)
	if err != nil {
		return nil, fmt.Errorf("build state request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send state request: %w", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxMockwayStateResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read state response: %w", err)
	}
	// Status check before size check so a non-2xx with a multi-MB body
	// surfaces with the upstream's body (truncated) rather than as an
	// unhelpful "payload exceeds 8 MB". Mirrors the API-side
	// httpMockStateClient.State.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch mock state: unexpected status %d: %s", resp.StatusCode, truncateMockwayErrorPayload(payload))
	}
	if len(payload) > maxMockwayStateResponseBytes {
		return nil, fmt.Errorf("read state response: payload exceeds %d bytes", maxMockwayStateResponseBytes)
	}

	return payload, nil
}

func (c *mockStateClient) postNoBody(ctx context.Context, path string, action string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build %s request: %w", action, err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send %s request: %w", action, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s: unexpected status %d: %s", action, resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	return nil
}

func truncateMockwayErrorPayload(payload []byte) string {
	trimmed := strings.TrimSpace(string(payload))
	if len(trimmed) <= maxMockwayErrorPayloadBytes {
		return trimmed
	}
	return trimmed[:maxMockwayErrorPayloadBytes] + "..."
}
