package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMockwayStateClientSetsHTTPTimeout(t *testing.T) {
	t.Parallel()

	client := newMockStateClient("http://localhost:8080")
	if client.client.Timeout != 30*time.Second {
		t.Fatalf("expected timeout 30s, got %s", client.client.Timeout)
	}
}

func TestMockwayStateClientStateReadsWithinBound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mock/state" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"mock":true}`))
	}))
	defer server.Close()

	client := newMockStateClient(server.URL)
	state, err := client.State(context.Background())
	if err != nil {
		t.Fatalf("state read: %v", err)
	}
	if string(state) != `{"mock":true}` {
		t.Fatalf("unexpected state payload: %q", string(state))
	}
}

func TestMockwayStateClientStateFailsWhenPayloadExceedsBound(t *testing.T) {
	t.Parallel()

	oversized := strings.Repeat("a", maxMockwayStateResponseBytes+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(oversized))
	}))
	defer server.Close()

	client := newMockStateClient(server.URL)
	_, err := client.State(context.Background())
	if err == nil {
		t.Fatal("expected payload limit error")
	}
	expected := "read state response: payload exceeds"
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected %q error, got %v", expected, err)
	}
}

func TestMockwayStateClientStateTruncatesErrorPayload(t *testing.T) {
	t.Parallel()

	oversized := strings.Repeat("x", maxMockwayErrorPayloadBytes+100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(oversized))
	}))
	defer server.Close()

	client := newMockStateClient(server.URL)
	_, err := client.State(context.Background())
	if err == nil {
		t.Fatal("expected status error")
	}
	if !strings.Contains(err.Error(), "...") {
		t.Fatalf("expected truncated payload marker, got %v", err)
	}
	if strings.Contains(err.Error(), oversized) {
		t.Fatalf("expected payload truncation, got %v", err)
	}
}

// TestCloudMockStateRouterResetHonoursS3AutoReset: with s3.auto_reset
// false, Reset and ResetAll send nothing to the s3 backend, so an AWS
// run needs no S3 backend up; with it true, both still empty the
// backend (the M59 BucketAlreadyExists fix).
func TestCloudMockStateRouterResetHonoursS3AutoReset(t *testing.T) {
	t.Parallel()

	fakeaws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer fakeaws.Close()

	var s3Hits atomic.Int32
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s3Hits.Add(1)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<ListAllMyBucketsResult><Buckets></Buckets></ListAllMyBucketsResult>`))
	}))
	defer s3.Close()

	resets := map[string]func(*cloudMockStateRouter) error{
		"Reset":    func(r *cloudMockStateRouter) error { return r.Reset(context.Background()) },
		"ResetAll": func(r *cloudMockStateRouter) error { return r.ResetAll(context.Background()) },
	}
	for name, reset := range resets {
		for _, autoReset := range []bool{false, true} {
			var cfg config.Config
			cfg.Mockway.URL = fakeaws.URL
			cfg.Fakeaws.URL = fakeaws.URL
			cfg.S3 = config.S3Config{URL: s3.URL, AutoReset: autoReset}
			runtime := &CommandRuntime{loadedScenario: &scenario.Scenario{Cloud: "aws"}}
			router := newCloudMockStateRouter(runtime, cfg)

			assert.Equal(t, autoReset, strings.HasSuffix(resetSummary(router), "+s3"), "summary must name s3 only when it is reset")
			s3Hits.Store(0)
			require.NoError(t, reset(router), "%s auto_reset=%t", name, autoReset)
			if autoReset {
				assert.Positive(t, s3Hits.Load(), "%s auto_reset=true must reset the s3 backend", name)
			} else {
				assert.Zero(t, s3Hits.Load(), "%s auto_reset=false must not touch the s3 backend", name)
			}
		}
	}
}
