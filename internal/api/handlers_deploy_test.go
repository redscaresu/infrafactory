package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
)

type fakeDeployer struct {
	result   ActionResult
	err      error
	calls    []string
	ttls     []string
	sawCtx   context.Context
	deadline time.Time
	hadDl    bool
	ctxErr   error
}

func (f *fakeDeployer) Deploy(ctx context.Context, name, ttl string) (ActionResult, error) {
	f.calls = append(f.calls, name)
	f.ttls = append(f.ttls, ttl)
	f.sawCtx = ctx
	f.ctxErr = ctx.Err()
	f.deadline, f.hadDl = ctx.Deadline()
	return f.result, f.err
}

func deployServer(t *testing.T, d DeploymentDeployer) *http.Server {
	t.Helper()
	return NewServer(ServerConfig{
		Config: config.Default(), Deployments: &fakeDeployments{}, Deployer: d,
	})
}

func postDeploy(t *testing.T, srv *http.Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/deployments", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	return rec
}

// ADR-0027: --allow-deploy is implied by neither --allow-layer3 nor
// --allow-teardown. An ephemeral apply the run destroys, destroying what
// exists, and creating what persists are three different kinds of harm.
func TestDeployDoesNotExistWithoutItsOwnFlag(t *testing.T) {
	// A server that CAN destroy still cannot create.
	srv := NewServer(ServerConfig{
		Config: config.Default(), Deployments: &fakeDeployments{},
		DeploymentActor: &fakeActor{},
	})

	rec := postDeploy(t, srv, `{"scenario":"lb-serving-paris"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "--allow-deploy")
}

func TestDeployCreatesTheNamedScenario(t *testing.T) {
	deployer := &fakeDeployer{result: ActionResult{Clean: true}}
	srv := deployServer(t, deployer)

	rec := postDeploy(t, srv, `{"scenario":"lb-serving-paris","ttl":"2h"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"lb-serving-paris"}, deployer.calls)
	assert.Equal(t, []string{"2h"}, deployer.ttls)
}

// A deploy that could not prove itself clean is not a 200, for the same
// reason a teardown is not: a page rendering a tick over "resources were
// created and could not be recorded" is the false green this project
// exists to avoid.
func TestDeployThatDidNotSucceedIsNotASuccess(t *testing.T) {
	srv := deployServer(t, &fakeDeployer{result: ActionResult{
		Clean: false,
		Failures: []ActionStep{{
			Stage: "deploy", Status: "fail",
			Detail: "resources may be live and could NOT be recorded",
		}},
	}})

	rec := postDeploy(t, srv, `{"scenario":"lb-serving-paris"}`)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "could NOT be recorded")
}

// An apply takes minutes and creates infrastructure as it goes. A client
// disconnecting halfway would leave resources with no completed record
// of what was made.
func TestDeploySurvivesTheClientDisconnecting(t *testing.T) {
	deployer := &fakeDeployer{result: ActionResult{Clean: true}}
	srv := deployServer(t, deployer)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/deployments",
		strings.NewReader(`{"scenario":"lb-serving-paris"}`)).WithContext(ctx)
	cancel()
	srv.Handler.ServeHTTP(httptest.NewRecorder(), req)

	// Recorded during the call: checking afterwards would measure the
	// handler's own defer cancel(), not the disconnect.
	assert.NoError(t, deployer.ctxErr, "the apply must still be running after the caller left")
	require.True(t, deployer.hadDl, "a detached apply still needs a backstop")
}

func TestDeployRequiresAScenario(t *testing.T) {
	deployer := &fakeDeployer{}
	srv := deployServer(t, deployer)

	rec := postDeploy(t, srv, `{"ttl":"2h"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, deployer.calls)
}

// The absences are the design. A request that could name a project could
// name somebody else's; run-owned projects are the harness's to create.
func TestDeployIgnoresAnythingItMustNotBeTold(t *testing.T) {
	deployer := &fakeDeployer{result: ActionResult{Clean: true}}
	srv := deployServer(t, deployer)

	rec := postDeploy(t, srv, `{
		"scenario":"lb-serving-paris",
		"project_id":"someone-elses-project",
		"skip_validation":true,
		"ttl":""
	}`)

	require.Equal(t, http.StatusOK, rec.Code)
	payload, err := json.Marshal(deployRequest{Scenario: "lb-serving-paris"})
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "project",
		"there is nowhere for a project to land, and that is the guarantee")
	assert.Equal(t, []string{""}, deployer.ttls, "an empty TTL means the scenario's own, never unbounded")
}

func TestDeployIsBehindTheOriginGuard(t *testing.T) {
	srv := deployServer(t, &fakeDeployer{result: ActionResult{Clean: true}})

	req := httptest.NewRequest(http.MethodPost, "/api/deployments",
		strings.NewReader(`{"scenario":"x"}`))
	req.Host = "127.0.0.1:4173"
	req.Header.Set("Origin", "https://evil.example")

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// Listing must keep working on a server that can also create.
func TestDeployDoesNotBreakTheListing(t *testing.T) {
	srv := deployServer(t, &fakeDeployer{})

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/deployments", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	var payload deploymentsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.True(t, payload.DeployAllowed)
	assert.False(t, payload.TeardownAllowed, "one capability does not imply the other")
}

// A client typo, or a UI holding a stale scenario list, is not a server
// fault. Answering 500 teaches operators that 500 means nothing in
// particular.
func TestDeployOfAnUnknownScenarioIsNotFound(t *testing.T) {
	srv := deployServer(t, &fakeDeployer{err: fmt.Errorf("no scenario named %q: %w", "gone", os.ErrNotExist)})

	rec := postDeploy(t, srv, `{"scenario":"gone"}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "no scenario named")
}

// A genuine fault is still a 500: the distinction only means something
// if both sides of it exist.
func TestDeployStillReportsARealFailureAsAServerError(t *testing.T) {
	srv := deployServer(t, &fakeDeployer{err: errors.New("the runtime could not be built")})

	rec := postDeploy(t, srv, `{"scenario":"lb-serving-paris"}`)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
