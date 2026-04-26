package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/redscaresu/infrafactory/internal/scenario"
)

// recordingMockServer accepts /mock/state, /mock/reset, /mock/snapshot,
// /mock/restore and records which paths it received. Used by the router
// tests below to confirm the cloudMockStateRouter dispatches to the
// expected backend per scenario.
type recordingMockServer struct {
	server *httptest.Server
	hits   int
	label  string
}

func newRecordingMockServer(t *testing.T, label string) *recordingMockServer {
	t.Helper()
	rec := &recordingMockServer{label: label}
	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.hits++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"label":"` + label + `"}`))
	}))
	t.Cleanup(rec.server.Close)
	return rec
}

// TestCloudMockStateRouterDispatchesByScenarioCloud guards the
// per-scenario dynamic dispatch: a scenario with `cloud: scaleway`
// must hit the mockway URL; `cloud: gcp` must hit the fakegcp URL.
// The harness layer captures one MockStateClient at construction
// time, so this test wires both endpoints up front and only changes
// `runtime.loadedScenario.Cloud` between calls.
func TestCloudMockStateRouterDispatchesByScenarioCloud(t *testing.T) {
	scaleway := newRecordingMockServer(t, "scaleway")
	gcp := newRecordingMockServer(t, "gcp")

	runtime := &CommandRuntime{}
	router := &cloudMockStateRouter{
		runtime:  runtime,
		scaleway: newMockStateClient(scaleway.server.URL),
		gcp:      newMockStateClient(gcp.server.URL),
	}

	cases := []struct {
		name        string
		scenario    *scenario.Scenario
		wantBackend string
	}{
		{name: "scaleway scenario hits mockway", scenario: &scenario.Scenario{Cloud: "scaleway"}, wantBackend: "scaleway"},
		{name: "gcp scenario hits fakegcp", scenario: &scenario.Scenario{Cloud: "gcp"}, wantBackend: "gcp"},
		{name: "uppercase GCP still hits fakegcp", scenario: &scenario.Scenario{Cloud: "GCP"}, wantBackend: "gcp"},
		{name: "unknown cloud falls back to scaleway", scenario: &scenario.Scenario{Cloud: "aws"}, wantBackend: "scaleway"},
		{name: "empty cloud falls back to scaleway", scenario: &scenario.Scenario{Cloud: ""}, wantBackend: "scaleway"},
		{name: "no scenario loaded falls back to scaleway", scenario: nil, wantBackend: "scaleway"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scaleway.hits = 0
			gcp.hits = 0
			runtime.loadedScenario = tc.scenario

			payload, err := router.State(context.Background())
			if err != nil {
				t.Fatalf("State: %v", err)
			}
			if !strings.Contains(string(payload), `"label":"`+tc.wantBackend+`"`) {
				t.Fatalf("expected payload from %s backend, got %s", tc.wantBackend, string(payload))
			}
			if tc.wantBackend == "scaleway" {
				if scaleway.hits != 1 || gcp.hits != 0 {
					t.Fatalf("expected 1 scaleway hit / 0 gcp, got scaleway=%d gcp=%d", scaleway.hits, gcp.hits)
				}
			} else {
				if scaleway.hits != 0 || gcp.hits != 1 {
					t.Fatalf("expected 0 scaleway / 1 gcp, got scaleway=%d gcp=%d", scaleway.hits, gcp.hits)
				}
			}
		})
	}
}

// TestCloudMockStateRouterFallsBackWhenGCPUnconfigured verifies that
// a GCP scenario falls back to the Scaleway client when no fakegcp
// URL is configured, rather than nil-panicking. This is the
// developer-machine common case before fakegcp is started.
func TestCloudMockStateRouterFallsBackWhenGCPUnconfigured(t *testing.T) {
	scaleway := newRecordingMockServer(t, "scaleway")
	runtime := &CommandRuntime{loadedScenario: &scenario.Scenario{Cloud: "gcp"}}
	router := &cloudMockStateRouter{
		runtime:  runtime,
		scaleway: newMockStateClient(scaleway.server.URL),
		gcp:      nil,
	}

	if _, err := router.State(context.Background()); err != nil {
		t.Fatalf("State: %v", err)
	}
	if scaleway.hits != 1 {
		t.Fatalf("expected GCP scenario to fall back to scaleway when GCP unconfigured; got %d hits", scaleway.hits)
	}
}

// TestCloudMockStateRouterDispatchesAllAdminMethods confirms the
// router doesn't accidentally only proxy State() — Reset, Snapshot,
// and Restore also pick the right backend. Uses goroutine-style
// per-method assertions to keep each call's blast radius narrow.
func TestCloudMockStateRouterDispatchesAllAdminMethods(t *testing.T) {
	scaleway := newRecordingMockServer(t, "scaleway")
	gcp := newRecordingMockServer(t, "gcp")
	runtime := &CommandRuntime{loadedScenario: &scenario.Scenario{Cloud: "gcp"}}
	router := &cloudMockStateRouter{
		runtime:  runtime,
		scaleway: newMockStateClient(scaleway.server.URL),
		gcp:      newMockStateClient(gcp.server.URL),
	}
	ctx := context.Background()

	if err := router.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if err := router.Snapshot(ctx); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if err := router.Restore(ctx); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := router.State(ctx); err != nil {
		t.Fatalf("State: %v", err)
	}
	if scaleway.hits != 0 {
		t.Fatalf("expected zero scaleway hits for gcp scenario, got %d", scaleway.hits)
	}
	if gcp.hits != 4 {
		t.Fatalf("expected 4 gcp hits (Reset+Snapshot+Restore+State), got %d", gcp.hits)
	}
}

// TestCloudMockStateRouterRespectsScenarioReassignment pins the
// dynamic-rebind contract: changing runtime.loadedScenario between
// calls switches the backend without rebuilding the router. This is
// what lets the harness layer capture the router once at construction
// and still get the right backend per scenario.
func TestCloudMockStateRouterRespectsScenarioReassignment(t *testing.T) {
	scaleway := newRecordingMockServer(t, "scaleway")
	gcp := newRecordingMockServer(t, "gcp")
	runtime := &CommandRuntime{}
	router := &cloudMockStateRouter{
		runtime:  runtime,
		scaleway: newMockStateClient(scaleway.server.URL),
		gcp:      newMockStateClient(gcp.server.URL),
	}
	ctx := context.Background()

	runtime.loadedScenario = &scenario.Scenario{Cloud: "scaleway"}
	_, _ = router.State(ctx)

	runtime.loadedScenario = &scenario.Scenario{Cloud: "gcp"}
	_, _ = router.State(ctx)

	runtime.loadedScenario = &scenario.Scenario{Cloud: "scaleway"}
	_, _ = router.State(ctx)

	if scaleway.hits != 2 || gcp.hits != 1 {
		t.Fatalf("expected scaleway=2 gcp=1 across the three swaps, got scaleway=%d gcp=%d", scaleway.hits, gcp.hits)
	}
}

