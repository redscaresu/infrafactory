package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/redscaresu/infrafactory/internal/config"
)

type fakeActor struct {
	result    ActionResult
	err       error
	teardowns []string
	reaps     int
	notExist  bool
}

func (f *fakeActor) Teardown(_ context.Context, id string) (ActionResult, error) {
	f.teardowns = append(f.teardowns, id)
	if f.notExist {
		return ActionResult{}, os.ErrNotExist
	}
	return f.result, f.err
}

func (f *fakeActor) Reap(context.Context) (ActionResult, error) {
	f.reaps++
	return f.result, f.err
}

func actionServer(t *testing.T, actor DeploymentActor) *http.Server {
	t.Helper()
	return NewServer(ServerConfig{
		Config: config.Default(), Deployments: &fakeDeployments{}, DeploymentActor: actor,
	})
}

func do(t *testing.T, srv *http.Server, method, path string) (*httptest.ResponseRecorder, ActionResult) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	var result ActionResult
	if rec.Code == http.StatusOK || rec.Code == http.StatusConflict {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	}
	return rec, result
}

// The capability does not EXIST unless the operator asked for it in the
// shell. Not "exists and refuses": a request must not be able to talk
// this server into destroying infrastructure, and that property survives
// a bug in the origin guard.
func TestTeardownEndpointsDoNotExistWithoutTheFlag(t *testing.T) {
	srv := actionServer(t, nil)

	for _, tc := range []struct{ method, path string }{
		{http.MethodDelete, "/api/deployments/dep-1"},
		{http.MethodPost, "/api/deployments/reap"},
	} {
		rec, _ := do(t, srv, tc.method, tc.path)
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s %s", tc.method, tc.path)
		assert.Contains(t, rec.Body.String(), "--allow-teardown")
	}
}

func TestTeardownDestroysTheNamedDeployment(t *testing.T) {
	actor := &fakeActor{result: ActionResult{Clean: true, Steps: []ActionStep{
		{Stage: "teardown", Status: "pass", Detail: "destroyed"},
	}}}
	srv := actionServer(t, actor)

	rec, result := do(t, srv, http.MethodDelete, "/api/deployments/dep-1")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, result.Clean)
	assert.Equal(t, []string{"dep-1"}, actor.teardowns)
}

// ADR-0024: a teardown that cannot PROVE the account clean must not
// report success. A page rendering a green tick over "the state file has
// vanished and the resources may still be running" is the false green
// this project exists to avoid.
func TestTeardownThatCannotProveCleanIsNotASuccess(t *testing.T) {
	srv := actionServer(t, &fakeActor{result: ActionResult{
		Clean: false,
		Failures: []ActionStep{{
			Stage: "teardown", Status: "fail",
			Detail: "state file has vanished; resources may still be running",
		}},
	}})

	rec, result := do(t, srv, http.MethodDelete, "/api/deployments/dep-1")

	assert.Equal(t, http.StatusConflict, rec.Code, "a partial teardown is not a 200")
	assert.False(t, result.Clean)
	require.Len(t, result.Failures, 1)
	assert.Contains(t, result.Failures[0].Detail, "may still be running")
}

func TestReapDestroysEverythingExpired(t *testing.T) {
	actor := &fakeActor{result: ActionResult{Clean: true}}
	srv := actionServer(t, actor)

	rec, _ := do(t, srv, http.MethodPost, "/api/deployments/reap")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, actor.reaps)
}

// A deployment id addresses a file in the live store. Anything that
// could climb out of it is refused BEFORE the store is asked, so a
// traversal attempt is never a lookup.
func TestTeardownRefusesIdsThatCouldEscapeTheStore(t *testing.T) {
	actor := &fakeActor{result: ActionResult{Clean: true}}
	srv := actionServer(t, actor)

	for _, id := range []string{"..", "../etc/passwd", "a/b", "a.json", ""} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/deployments/"+id, nil)
		srv.Handler.ServeHTTP(rec, req)

		assert.NotEqual(t, http.StatusOK, rec.Code, "id %q must not be served", id)
	}
	assert.Empty(t, actor.teardowns, "the store must never be asked about these")
}

func TestTeardownOfAnUnknownDeploymentIsNotFound(t *testing.T) {
	srv := actionServer(t, &fakeActor{notExist: true})

	rec, _ := do(t, srv, http.MethodDelete, "/api/deployments/dep-missing")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// GET must not destroy, and neither must anything else that is not the
// one verb each action is spelled with.
func TestDeploymentActionsRefuseTheWrongVerb(t *testing.T) {
	actor := &fakeActor{result: ActionResult{Clean: true}}
	srv := actionServer(t, actor)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/deployments/dep-1"},
		{http.MethodPost, "/api/deployments/dep-1"},
		{http.MethodGet, "/api/deployments/reap"},
		{http.MethodDelete, "/api/deployments/reap"},
	} {
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		assert.Equal(t, http.StatusMethodNotAllowed, rec.Code, "%s %s", tc.method, tc.path)
	}
	assert.Empty(t, actor.teardowns)
	assert.Zero(t, actor.reaps)
}

// The origin guard covers these too, by position rather than by anyone
// remembering (S160a).
func TestDeploymentActionsAreBehindTheOriginGuard(t *testing.T) {
	srv := actionServer(t, &fakeActor{result: ActionResult{Clean: true}})

	req := httptest.NewRequest(http.MethodDelete, "/api/deployments/dep-1", nil)
	req.Host = "127.0.0.1:4173"
	req.Header.Set("Origin", "https://evil.example")

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// A teardown cancelled halfway has deleted some resources and not
// others, and the live record then describes neither the old state nor
// the new one. Closing a browser tab must not do that.
//
// The same rule ensureRunProject applies to creating a run's project.
func TestTeardownSurvivesTheClientDisconnecting(t *testing.T) {
	actor := &contextCapturingActor{result: ActionResult{Clean: true}}
	srv := actionServer(t, actor)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodDelete, "/api/deployments/dep-1", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	// The client goes away before the handler runs.
	cancel()
	srv.Handler.ServeHTTP(rec, req)

	// Recorded DURING the call. Checking the context afterwards would
	// measure the handler's own `defer cancel()`, not the client's
	// disconnect -- a distinction that made an earlier version of this
	// test fail against correct code.
	assert.NoError(t, actor.errDuringCall, "the destroy must still be running after the caller left")
	assert.Equal(t, http.StatusOK, rec.Code)

	require.True(t, actor.hadDeadline, "a detached destroy still needs a backstop against a hung provider call")
	assert.True(t, actor.deadline.After(time.Now().Add(20*time.Minute)),
		"the backstop must not cut a real destroy short")
}

func TestReapSurvivesTheClientDisconnecting(t *testing.T) {
	actor := &contextCapturingActor{result: ActionResult{Clean: true}}
	srv := actionServer(t, actor)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/deployments/reap", nil).WithContext(ctx)
	cancel()
	srv.Handler.ServeHTTP(httptest.NewRecorder(), req)

	assert.NoError(t, actor.errDuringCall)
}

type contextCapturingActor struct {
	result        ActionResult
	errDuringCall error
	deadline      time.Time
	hadDeadline   bool
}

func (c *contextCapturingActor) observe(ctx context.Context) {
	c.errDuringCall = ctx.Err()
	c.deadline, c.hadDeadline = ctx.Deadline()
}

func (c *contextCapturingActor) Teardown(ctx context.Context, _ string) (ActionResult, error) {
	c.observe(ctx)
	return c.result, nil
}

func (c *contextCapturingActor) Reap(ctx context.Context) (ActionResult, error) {
	c.observe(ctx)
	return c.result, nil
}
