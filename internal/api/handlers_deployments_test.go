package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
	"github.com/redscaresu/infrafactory/internal/livestore"
)

type fakeDeployments struct {
	deployments []livestore.Deployment
	unreadable  []error
	err         error
}

func (f *fakeDeployments) List() ([]livestore.Deployment, []error, error) {
	return f.deployments, f.unreadable, f.err
}

func getDeployments(t *testing.T, lister DeploymentLister) (*httptest.ResponseRecorder, deploymentsResponse) {
	t.Helper()
	srv := NewServer(ServerConfig{Config: config.Default(), Deployments: lister})
	req := httptest.NewRequest(http.MethodGet, "/api/deployments", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	var payload deploymentsResponse
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	}
	return rec, payload
}

func liveDeployment(id string, ttl time.Duration, observations ...livestore.Observation) livestore.Deployment {
	return livestore.Deployment{
		ID: id, Scenario: "web-live-paris", Cloud: "scaleway",
		ProjectID: "proj-1", State: livestore.StateLive,
		CreatedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(ttl),
		Observations: observations,
	}
}

// The load-bearing case for the estate page. A deployment nobody probed
// must arrive saying so, in words, on both axes -- a blank cell beside a
// `confirmed` one invites the reader to read silence as health.
func TestDeploymentsReportUnobservedAndUncheckedInWords(t *testing.T) {
	_, payload := getDeployments(t, &fakeDeployments{
		deployments: []livestore.Deployment{liveDeployment("dep-1", time.Hour)},
	})

	require.Len(t, payload.Deployments, 1)
	assert.Equal(t, livestore.HealthUnobserved, payload.Deployments[0].Health.Status)
	assert.Equal(t, "unchecked", payload.Deployments[0].Health.Version)
}

// A record that will not decode may describe running, billing
// infrastructure. `live ls` exits non-zero for this; a GET cannot, so the
// payload has to carry it where a page will see it.
func TestDeploymentsCarryRecordsTheStoreCouldNotRead(t *testing.T) {
	_, payload := getDeployments(t, &fakeDeployments{
		unreadable: []error{errors.New("dep-broken.json: unexpected end of JSON input")},
	})

	require.Len(t, payload.Unreadable, 1)
	assert.Contains(t, payload.Unreadable[0], "dep-broken.json")
}

// Healthy-but-on-the-wrong-version is the most dangerous state the system
// can be in, and the one every other signal calls fine.
func TestDeploymentsKeepVersionDriftVisible(t *testing.T) {
	_, payload := getDeployments(t, &fakeDeployments{
		deployments: []livestore.Deployment{liveDeployment("dep-1", time.Hour, livestore.Observation{
			At: time.Now(), Status: livestore.ObservationHealthy, Version: livestore.VersionUnconfirmed,
		})},
	})

	require.Len(t, payload.Deployments, 1)
	assert.Equal(t, "healthy", payload.Deployments[0].Health.Status)
	assert.Equal(t, "unconfirmed", payload.Deployments[0].Health.Version)
}

// What is about to vanish needs attention most.
func TestDeploymentsAreOrderedBySoonestToExpire(t *testing.T) {
	_, payload := getDeployments(t, &fakeDeployments{deployments: []livestore.Deployment{
		liveDeployment("dep-late", 4*time.Hour),
		liveDeployment("dep-soon", time.Minute),
		liveDeployment("dep-mid", time.Hour),
	}})

	require.Len(t, payload.Deployments, 3)
	assert.Equal(t, []string{"dep-soon", "dep-mid", "dep-late"}, []string{
		payload.Deployments[0].ID, payload.Deployments[1].ID, payload.Deployments[2].ID,
	})
}

// An upgraded deployment is a different thing from one that never moved.
func TestDeploymentsMarkOnesThatWereUpgraded(t *testing.T) {
	d := liveDeployment("dep-1", time.Hour)
	d.UpgradedAt = time.Now()

	_, payload := getDeployments(t, &fakeDeployments{deployments: []livestore.Deployment{d}})

	require.Len(t, payload.Deployments, 1)
	assert.True(t, payload.Deployments[0].Upgraded)
}

// Read-only, deliberately: deploy, teardown and reap carry guards that
// live in internal/cli and are not reachable from here.
func TestDeploymentsRefuseEveryMutatingMethod(t *testing.T) {
	srv := NewServer(ServerConfig{Config: config.Default(), Deployments: &fakeDeployments{}})

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/deployments", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, "%s must not be served", method)
	}
}

// A store that cannot be read is an error, never an empty estate: the
// same rule live reconcile applies, for the same reason.
func TestDeploymentsFailRatherThanReportingAnEmptyEstate(t *testing.T) {
	rec, _ := getDeployments(t, &fakeDeployments{err: errors.New("permission denied")})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// A server with no live store configured says so, rather than answering
// with an empty list that reads as "nothing is running".
func TestDeploymentsSayWhenTheyAreNotConfigured(t *testing.T) {
	srv := NewServer(ServerConfig{Config: config.Default()})
	req := httptest.NewRequest(http.MethodGet, "/api/deployments", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

// The class, closed once rather than field by field.
//
// `omitempty` does not omit a zero `time.Time` -- it is a struct, so the
// tag has nothing to act on and the field marshals as year 1. Every
// optional time in this payload has that defect by default, and the next
// one added to the record inherits it silently.
//
// Three review findings in this slice were instances of it: the version
// label, the observation timestamp, and the upgrade timestamps. Checking
// the field in front of me and not its neighbours produced all three, so
// this checks the whole payload instead.
func TestDeploymentPayloadNeverCarriesAYearOneTimestamp(t *testing.T) {
	// Deliberately a record with every optional field unset: never
	// upgraded, never observed, no address, no image.
	rec, _ := getDeployments(t, &fakeDeployments{
		deployments: []livestore.Deployment{{
			ID: "dep-bare", Scenario: "web-live-paris", Cloud: "scaleway",
			ProjectID: "proj-1", State: livestore.StateLive,
			CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
		}},
	})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "0001-01-01",
		"an absent moment must serialise as null; a page renders a date and a reader trusts it")
}
