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
// either backend. Per-scenario cloud dispatch happens at the
// cloudMockStateRouter layer below.
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

// cloudMockStateRouter dispatches MockStateClient calls between the
// Scaleway mock (mockway), GCP mock (fakegcp), and AWS mock (fakeaws)
// based on the currently-loaded scenario's cloud. The harness layer
// captures one MockStateClient at construction time, so a single
// shared router keeps that capture valid even when the scenario (and
// therefore the target backend) changes between runs.
//
// Per concepts.md "Required surface" item 16 (S43-T9): per-cloud
// reset/snapshot/restore — an aws scenario's reset only touches
// fakeaws, not mockway or fakegcp. pick() enforces this.
//
// Third-party mock carve-out (M59): when an AWS scenario is loaded
// AND r.s3 is configured, S3 calls fan out to the third-party S3
// backend (SeaweedFS by default — Apache 2.0, full S3 surface)
// instead of fakeaws's stripped-down S3 handler. The cloud-default
// client (fakeaws) still handles everything else. Reset fans out
// to both fakeaws AND the s3 backend; State() merges the s3-derived
// S3 block into the fakeaws state (see internal/cli/s3_state.go).
type cloudMockStateRouter struct {
	runtime  *CommandRuntime
	scaleway *mockStateClient
	gcp      *mockStateClient
	aws      *mockStateClient
	genesys  *mockStateClient // S114-T5: Genesys Cloud CCaaS mock
	s3       *mockStateClient // S3 carve-out for AWS scenarios (M59)
}

func (r *cloudMockStateRouter) Reset(ctx context.Context) error {
	if err := r.pick("").Reset(ctx); err != nil {
		return err
	}
	if extra := r.pick("s3"); extra != nil && extra != r.pick("") {
		// SeaweedFS / similar third-party S3 backends have no
		// /mock/reset endpoint — go through the native S3 admin
		// path (list+delete buckets).
		return resetS3Backend(ctx, extra)
	}
	return nil
}

func (r *cloudMockStateRouter) Snapshot(ctx context.Context) error {
	if err := r.pick("").Snapshot(ctx); err != nil {
		return err
	}
	// Third-party S3 backends have no native snapshot — documented
	// gap in CONCEPT.md "Third-Party Mock Integration" §6.
	return nil
}

func (r *cloudMockStateRouter) Restore(ctx context.Context) error {
	if err := r.pick("").Restore(ctx); err != nil {
		return err
	}
	// Third-party S3 backends have no native restore — see Snapshot.
	return nil
}

func (r *cloudMockStateRouter) State(ctx context.Context) ([]byte, error) {
	base, err := r.pick("").State(ctx)
	if err != nil {
		return nil, err
	}
	if r.isAWSScenario() && r.s3 != nil {
		return mergeS3IntoAWSState(ctx, base, r.s3)
	}
	return base, nil
}

// pick resolves to the per-cloud client based on the loaded scenario's
// cloud field AND the optional service hint:
//   - service "s3" + cloud:aws → r.s3 (when configured)
//   - cloud:gcp → r.gcp (when configured)
//   - cloud:aws → r.aws (when configured)
//   - default / scaleway / pre-LoadScenario → r.scaleway
//
// When a cloud is named but its client URL is not configured (URL ==
// "" → r.X is nil), we fall back to scaleway. This matches the legacy
// fakegcp fallback behaviour and keeps the runtime constructible
// when only a subset of clouds is wired.
//
// Pass `service=""` for the cloud-default; pass a specific service
// name (e.g. "s3") when the call is service-scoped — used by the
// fan-out Reset/State logic above.
func (r *cloudMockStateRouter) pick(service string) *mockStateClient {
	if service == "s3" && r.isAWSScenario() && r.s3 != nil {
		return r.s3
	}
	if r.runtime != nil && r.runtime.loadedScenario != nil {
		switch strings.ToLower(strings.TrimSpace(r.runtime.loadedScenario.Cloud)) {
		case "gcp":
			if r.gcp != nil {
				return r.gcp
			}
		case "aws":
			if r.aws != nil {
				return r.aws
			}
		case "genesys":
			if r.genesys != nil {
				return r.genesys
			}
		}
	}
	return r.scaleway
}

// isAWSScenario reports whether the loaded scenario targets AWS —
// used by the M59 S3 carve-out logic to decide whether the s3 backend
// should be consulted.
func (r *cloudMockStateRouter) isAWSScenario() bool {
	if r.runtime == nil || r.runtime.loadedScenario == nil {
		return false
	}
	return strings.ToLower(strings.TrimSpace(r.runtime.loadedScenario.Cloud)) == "aws"
}

// ResetAll resets every configured mock backend independently of the
// loaded scenario, so a sweep harness (or interactive `infrafactory
// mock reset`) can drop accumulated state in one call. Hits mockway,
// fakegcp, fakeaws (each when configured) and cascades to the s3
// backend (SeaweedFS by default) via resetS3Backend.
//
// Motivated by the S54 SeaweedFS state-leak: bare-curl harnesses
// hitting `/mock/reset` on fakeaws don't touch SeaweedFS, leaving
// pre-sweep buckets that caused `BucketAlreadyExists` on aws-full-stack.
// Routing through this method instead is the systematic fix.
//
// Returns the first error encountered, but attempts every backend
// before returning so partial resets still happen.
func (r *cloudMockStateRouter) ResetAll(ctx context.Context) error {
	var firstErr error
	for _, c := range []*mockStateClient{r.scaleway, r.gcp, r.aws, r.genesys} {
		if c == nil {
			continue
		}
		if err := c.Reset(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if r.s3 != nil {
		if err := resetS3Backend(ctx, r.s3); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
