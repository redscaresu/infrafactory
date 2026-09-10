package harness

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeNICAPI struct {
	project   string
	servers   []nicServer
	nicsBySrv map[string][]privateNIC
	requests  []string
	deleted   map[string]bool
}

func (f *fakeNICAPI) do(req *http.Request) (*http.Response, error) {
	f.requests = append(f.requests, req.Method+" "+req.URL.Path)
	reply := func(code int, body any) (*http.Response, error) {
		b, _ := json.Marshal(body)
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(string(b)))}, nil
	}
	switch {
	case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/servers"):
		if !strings.Contains(req.URL.Path, "/zones/fr-par-1/") {
			return reply(http.StatusOK, map[string]any{"servers": []nicServer{}})
		}
		return reply(http.StatusOK, map[string]any{"servers": f.servers})
	case req.Method == http.MethodGet && strings.HasSuffix(req.URL.Path, "/private_nics"):
		sid := strings.Split(req.URL.Path, "/servers/")[1]
		sid = strings.TrimSuffix(sid, "/private_nics")
		return reply(http.StatusOK, map[string]any{"private_nics": f.nicsBySrv[sid]})
	case req.Method == http.MethodDelete && strings.Contains(req.URL.Path, "/private_nics/"):
		if f.deleted == nil {
			f.deleted = map[string]bool{}
		}
		f.deleted[req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]] = true
		return reply(http.StatusNoContent, map[string]any{})
	}
	return reply(http.StatusNotFound, map[string]any{})
}

func apiWithNIC(project, serverID, srvProject, nicID string) *fakeNICAPI {
	return &fakeNICAPI{
		project:   project,
		servers:   []nicServer{{ID: serverID, Project: srvProject}},
		nicsBySrv: map[string][]privateNIC{serverID: {{ID: nicID}}},
	}
}

func TestPrivateNICDetachRemovesNICsInTheProject(t *testing.T) {
	const project = "11111111-1111-1111-1111-111111111111"
	api := apiWithNIC(project, "srv-a", project, "nic-a")

	d := NewScalewayPrivateNICDetachWithDoer("https://api.example", api.do)
	removed, err := d.Run(context.Background(), project, "secret")

	require.NoError(t, err)
	require.Len(t, removed, 1)
	assert.Contains(t, removed[0], "nic-a")
	assert.True(t, api.deleted["nic-a"])
}

// The v1 route is the entire point. v2alpha1 is what the provider calls
// and what answers 412 for every NIC that exists -- swapping this for
// the newer-looking endpoint would reintroduce the defect wholesale.
func TestPrivateNICDetachUsesTheV1Route(t *testing.T) {
	const project = "11111111-1111-1111-1111-111111111111"
	api := apiWithNIC(project, "srv-a", project, "nic-a")

	d := NewScalewayPrivateNICDetachWithDoer("https://api.example", api.do)
	_, err := d.Run(context.Background(), project, "secret")
	require.NoError(t, err)

	var del string
	for _, r := range api.requests {
		if strings.HasPrefix(r, http.MethodDelete) {
			del = r
		}
	}
	assert.Contains(t, del, "/instance/v1/zones/fr-par-1/servers/srv-a/private_nics/nic-a")
	assert.NotContains(t, del, "v2alpha1", "v2alpha1 refuses every NIC; using it is the bug")
}

// Defence in depth, as in the purge: the credential sees the whole
// organization, so a query parameter is not allowed to be the only thing
// keeping this off another project's servers.
func TestPrivateNICDetachIgnoresServersOutsideTheProject(t *testing.T) {
	const project = "11111111-1111-1111-1111-111111111111"
	api := apiWithNIC(project, "srv-theirs", "99999999-9999-9999-9999-999999999999", "nic-theirs")

	d := NewScalewayPrivateNICDetachWithDoer("https://api.example", api.do)
	removed, err := d.Run(context.Background(), project, "secret")

	require.NoError(t, err)
	assert.Empty(t, removed)
	assert.False(t, api.deleted["nic-theirs"], "another project's NIC must not be touched")
}

func TestPrivateNICDetachRefusesAnEmptyProject(t *testing.T) {
	d := NewScalewayPrivateNICDetachWithDoer("https://api.example", func(*http.Request) (*http.Response, error) {
		return nil, assert.AnError
	})
	_, err := d.Run(context.Background(), "", "secret")
	require.Error(t, err)
}
