package harness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const purgeProject = "6c4390c9-664e-4289-a34f-cdc865653fc7"

// purgeServer answers the two calls the purge makes. Only fr-par-1 holds
// anything; every other zone returns an empty list, which is what the
// real API does and what the walk over InstanceZones has to tolerate.
func purgeServer(t *testing.T, body string, deleted *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/security_groups"):
			if !strings.Contains(r.URL.Path, "fr-par-1") {
				_, _ = w.Write([]byte(`{"security_groups":[]}`))
				return
			}
			assert.Equal(t, purgeProject, r.URL.Query().Get("project"),
				"purge must scope its list to the run's project")
			_, _ = w.Write([]byte(body))
		case r.Method == http.MethodDelete:
			parts := strings.Split(r.URL.Path, "/")
			*deleted = append(*deleted, parts[len(parts)-1])
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
}

// The whole reason this type exists: Scaleway auto-creates a default
// security group in a fresh project, and the project cannot be deleted
// while it is there.
func TestPurgeRemovesProjectDefaultSecurityGroup(t *testing.T) {
	var deleted []string
	srv := purgeServer(t, `{"security_groups":[
		{"id":"142eef7b","name":"Default security group","project_default":true,"project":"6c4390c9-664e-4289-a34f-cdc865653fc7"}]}`, &deleted)
	defer srv.Close()

	purge := NewScalewayAutoCreatedPurgeWithDoer(srv.URL, srv.Client().Do)
	removed, err := purge.Run(context.Background(), purgeProject, "secret")

	require.NoError(t, err)
	assert.Equal(t, []string{"142eef7b"}, deleted)
	require.Len(t, removed, 1)
	assert.Contains(t, removed[0], "142eef7b")
}

// A group the run's own HCL created is Terraform's to destroy. Deleting
// it here would paper over a real destroy bug.
func TestPurgeLeavesNonDefaultSecurityGroupsAlone(t *testing.T) {
	var deleted []string
	srv := purgeServer(t, `{"security_groups":[
		{"id":"declared-by-hcl","name":"web","project_default":false,"project":"6c4390c9-664e-4289-a34f-cdc865653fc7"}]}`, &deleted)
	defer srv.Close()

	purge := NewScalewayAutoCreatedPurgeWithDoer(srv.URL, srv.Client().Do)
	removed, err := purge.Run(context.Background(), purgeProject, "secret")

	require.NoError(t, err)
	assert.Empty(t, deleted, "only API-auto-created resources may be purged")
	assert.Empty(t, removed)
}

// Without a project to scope to, the purge has no blast radius it can
// prove, so it refuses rather than guessing.
func TestPurgeRefusesWithoutAProject(t *testing.T) {
	purge := NewScalewayAutoCreatedPurgeWithDoer("http://127.0.0.1:1", http.DefaultClient.Do)

	_, err := purge.Run(context.Background(), "", "secret")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "project id")
}

// Lenient by design: the orphan sweep is what fails closed. A list error
// must not turn into a failed teardown.
func TestPurgeToleratesUnreachableZones(t *testing.T) {
	purge := NewScalewayAutoCreatedPurgeWithDoer("http://127.0.0.1:1", http.DefaultClient.Do)

	removed, err := purge.Run(context.Background(), purgeProject, "secret")

	require.NoError(t, err)
	assert.Empty(t, removed)
}

// Defence in depth for destructive code: the API is asked to filter by
// project, and the response is checked anyway. The credential can see
// every project in the organization, so a changed filter name or an
// unfiltered page would otherwise be enough to delete another project's
// default security group.
func TestPurgeRefusesGroupsOutsideTheRunProject(t *testing.T) {
	var deleted []string
	srv := purgeServer(t, `{"security_groups":[
		{"id":"someone-elses","name":"Default security group","project_default":true,
		 "project":"0b6a8a6a-7242-4852-a0cb-ac2e4fc86b92"},
		{"id":"ours","name":"Default security group","project_default":true,
		 "project":"6c4390c9-664e-4289-a34f-cdc865653fc7"}]}`, &deleted)
	defer srv.Close()

	purge := NewScalewayAutoCreatedPurgeWithDoer(srv.URL, srv.Client().Do)
	removed, err := purge.Run(context.Background(), purgeProject, "secret")

	require.NoError(t, err)
	assert.Equal(t, []string{"ours"}, deleted,
		"a group the API returned but that belongs to another project must not be deleted")
	require.Len(t, removed, 1)
}

// A response with no project field is not evidence of membership.
func TestPurgeRefusesGroupsWithNoProject(t *testing.T) {
	var deleted []string
	srv := purgeServer(t, `{"security_groups":[
		{"id":"unattributed","name":"Default security group","project_default":true}]}`, &deleted)
	defer srv.Close()

	purge := NewScalewayAutoCreatedPurgeWithDoer(srv.URL, srv.Client().Do)
	removed, err := purge.Run(context.Background(), purgeProject, "secret")

	require.NoError(t, err)
	assert.Empty(t, deleted)
	assert.Empty(t, removed)
}

// The first version of this list was written from memory and omitted
// it-mil-1, a real Instance zone -- an instance placed there would have
// left its project undeletable. Pin the enumeration so the next edit is
// deliberate.
func TestInstanceZonesCoverEveryScalewayInstanceZone(t *testing.T) {
	assert.ElementsMatch(t, []string{
		"fr-par-1", "fr-par-2", "fr-par-3",
		"nl-ams-1", "nl-ams-2", "nl-ams-3",
		"pl-waw-1", "pl-waw-2", "pl-waw-3",
		"it-mil-1",
	}, InstanceZones, "verified against the live API on 2026-08-24; see the InstanceZones doc comment to re-verify")
}
