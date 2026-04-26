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

// newMockStateClient builds a multi-cloud-capable HTTP client for the
// `/mock/{state,reset,snapshot,restore}` admin endpoints exposed by both
// mockway (Scaleway) and fakegcp (GCP). The endpoint shapes are identical
// across the two backends, so the same client serves either provider —
// callers select the URL based on the scenario's cloud.
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
	if len(payload) > maxMockwayStateResponseBytes {
		return nil, fmt.Errorf("read state response: payload exceeds %d bytes", maxMockwayStateResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch mock state: unexpected status %d: %s", resp.StatusCode, truncateMockwayErrorPayload(payload))
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
