package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ministackClient implements the stateClient interface for the
// ministackorg/ministack open-source AWS emulator that's superseding
// fakeaws in M64+. ministack uses a different admin-endpoint
// convention than mockway/fakegcp/fakeaws — `/_ministack/reset`
// instead of `/mock/reset`, and no native `/mock/state` (the polyfill
// in ministack_state.go walks AWS-SDK introspection APIs to synthesize
// the expected JSON shape; M65). Snapshot / Restore are not modelled
// upstream and return nil (documented gap; the only consumer is the
// `--no-destroy` incremental-run path which falls back to clean runs
// when snapshot/restore are unavailable).
type ministackClient struct {
	baseURL string
	client  *http.Client

	// stateBuilder synthesizes the /mock/state JSON shape by walking
	// ministack's AWS-SDK introspection APIs. Wired in M65; for the
	// M64 baseline this is nil and State() returns an explicit
	// "polyfill not yet wired" error so the failure mode is loud.
	stateBuilder ministackStateBuilder
}

// ministackStateBuilder is the M65 polyfill seam — implementations
// take a ministack base URL and emit a JSON blob in the same shape
// that fakeaws's GET /mock/state returns. Kept as an interface so
// the polyfill can be added in M65 without re-touching this file.
type ministackStateBuilder interface {
	BuildState(ctx context.Context, baseURL string) ([]byte, error)
}

func newMinistackClient(baseURL string) *ministackClient {
	return &ministackClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (m *ministackClient) Reset(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/_ministack/reset", nil)
	if err != nil {
		return fmt.Errorf("build ministack reset request: %w", err)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("send ministack reset request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("ministack reset: unexpected status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	return nil
}

// Snapshot is a no-op for ministack — the upstream image doesn't
// model snapshot/restore. The runtime's `--no-destroy` incremental
// path expects a Snapshot/Restore pair that round-trips state; since
// ministack can't, callers that need incremental restoration fall back
// to clean-mode runs. Returning nil instead of erroring keeps the
// common-case Reset+Apply flow working.
func (m *ministackClient) Snapshot(ctx context.Context) error {
	return nil
}

// Restore is a no-op for the same reason as Snapshot.
func (m *ministackClient) Restore(ctx context.Context) error {
	return nil
}

// State delegates to the M65 polyfill (ministack_state.go::*Builder).
// Until M65 lands the builder field is nil and we surface an explicit
// error rather than masquerading as success.
func (m *ministackClient) State(ctx context.Context) ([]byte, error) {
	if m.stateBuilder == nil {
		return nil, fmt.Errorf("ministack state polyfill not wired (M65 pending)")
	}
	return m.stateBuilder.BuildState(ctx, m.baseURL)
}
