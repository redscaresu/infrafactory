package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeInstanceAPI answers the three calls the poweroff makes, and records
// them. Servers start `running` and flip to `stopped` only after a
// poweroff action, which is what lets the wait be tested rather than
// assumed.
type fakeInstanceAPI struct {
	mu       sync.Mutex
	state    map[string]string // server id -> state
	project  map[string]string // server id -> project
	zone     string
	requests []string
	// poweroffsUntilStopped makes a server take N state polls to settle,
	// so "returned as soon as the action was accepted" fails the test.
	pollsBeforeStopped map[string]int
}

func (f *fakeInstanceAPI) do(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req.Method+" "+req.URL.Path)

	reply := func(code int, body any) (*http.Response, error) {
		b, _ := json.Marshal(body)
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	}

	path := req.URL.Path
	switch {
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/servers"):
		if !strings.Contains(path, "/zones/"+f.zone+"/") {
			return reply(http.StatusOK, map[string]any{"servers": []instanceServer{}})
		}
		var out []instanceServer
		for id, st := range f.state {
			out = append(out, instanceServer{ID: id, Name: "n-" + id, Project: f.project[id], State: st})
		}
		return reply(http.StatusOK, map[string]any{"servers": out})

	case req.Method == http.MethodPost && strings.HasSuffix(path, "/action"):
		id := strings.Split(path, "/servers/")[1]
		id = strings.TrimSuffix(id, "/action")
		body, _ := io.ReadAll(req.Body)
		if !strings.Contains(string(body), `"poweroff"`) {
			return reply(http.StatusBadRequest, map[string]any{})
		}
		f.state[id] = "stopping"
		return reply(http.StatusAccepted, map[string]any{})

	case req.Method == http.MethodGet && strings.Contains(path, "/servers/"):
		id := strings.Split(path, "/servers/")[1]
		if n := f.pollsBeforeStopped[id]; n > 0 {
			f.pollsBeforeStopped[id] = n - 1
		} else if f.state[id] == "stopping" {
			f.state[id] = "stopped"
		}
		return reply(http.StatusOK, map[string]any{"server": instanceServer{ID: id, State: f.state[id]}})
	}
	return reply(http.StatusNotFound, map[string]any{})
}

func newFakeInstanceAPI(zone string, servers map[string]string, project string) *fakeInstanceAPI {
	f := &fakeInstanceAPI{
		state: servers, project: map[string]string{}, zone: zone,
		pollsBeforeStopped: map[string]int{},
	}
	for id := range servers {
		f.project[id] = project
	}
	return f
}

func TestInstancePowerOffStopsRunningServersInTheProject(t *testing.T) {
	const project = "11111111-1111-1111-1111-111111111111"
	api := newFakeInstanceAPI("fr-par-1", map[string]string{"srv-a": "running"}, project)

	p := NewScalewayInstancePowerOffWithDoer("https://api.example", api.do)
	stopped, err := p.Run(context.Background(), project, "secret")

	require.NoError(t, err)
	require.Len(t, stopped, 1)
	assert.Contains(t, stopped[0], "srv-a")
	assert.Equal(t, "stopped", api.state["srv-a"])
}

// The wait is the whole point: `poweroff` returns when the task is
// accepted, and a destroy started then hits the very precondition this
// clears. A server that needs several polls must still come back stopped.
func TestInstancePowerOffWaitsForStopped(t *testing.T) {
	const project = "11111111-1111-1111-1111-111111111111"
	api := newFakeInstanceAPI("fr-par-1", map[string]string{"srv-slow": "running"}, project)
	api.pollsBeforeStopped["srv-slow"] = 5

	p := NewScalewayInstancePowerOffWithDoer("https://api.example", api.do)
	stopped, err := p.Run(context.Background(), project, "secret")

	require.NoError(t, err)
	require.Len(t, stopped, 1)
	assert.NotContains(t, stopped[0], "did NOT reach stopped")
	assert.Equal(t, "stopped", api.state["srv-slow"])
}

// A server that never settles must be REPORTED, not silently treated as
// stopped. It is the case where the destroy is about to fail.
func TestInstancePowerOffReportsAServerThatNeverStops(t *testing.T) {
	const project = "11111111-1111-1111-1111-111111111111"
	api := newFakeInstanceAPI("fr-par-1", map[string]string{"srv-stuck": "running"}, project)
	api.pollsBeforeStopped["srv-stuck"] = 10_000

	p := NewScalewayInstancePowerOffWithDoer("https://api.example", api.do)
	stopped, err := p.Run(context.Background(), project, "secret")

	require.NoError(t, err)
	require.Len(t, stopped, 1)
	assert.Contains(t, stopped[0], "did NOT reach stopped")
}

// Defence in depth, the same as the purge has. The credential can see
// every project in the organization, so a query parameter is not on its
// own allowed to be what keeps this off another project's servers.
func TestInstancePowerOffIgnoresServersOutsideTheProject(t *testing.T) {
	const project = "11111111-1111-1111-1111-111111111111"
	api := newFakeInstanceAPI("fr-par-1", map[string]string{"srv-theirs": "running"}, project)
	api.project["srv-theirs"] = "99999999-9999-9999-9999-999999999999"

	p := NewScalewayInstancePowerOffWithDoer("https://api.example", api.do)
	stopped, err := p.Run(context.Background(), project, "secret")

	require.NoError(t, err)
	assert.Empty(t, stopped, "a server in another project must not be touched")
	assert.Equal(t, "running", api.state["srv-theirs"], "and must not be powered off")
}

func TestInstancePowerOffSkipsAlreadyStoppedServers(t *testing.T) {
	const project = "11111111-1111-1111-1111-111111111111"
	api := newFakeInstanceAPI("fr-par-1", map[string]string{"srv-off": "stopped"}, project)

	p := NewScalewayInstancePowerOffWithDoer("https://api.example", api.do)
	stopped, err := p.Run(context.Background(), project, "secret")

	require.NoError(t, err)
	assert.Empty(t, stopped)
	for _, r := range api.requests {
		assert.NotContains(t, r, "/action", "an already-stopped server needs no poweroff")
	}
}

func TestInstancePowerOffRefusesAnEmptyProject(t *testing.T) {
	p := NewScalewayInstancePowerOffWithDoer("https://api.example", func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("must not be called")
	})
	_, err := p.Run(context.Background(), "", "secret")
	require.Error(t, err)
}
