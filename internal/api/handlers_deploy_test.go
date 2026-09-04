package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// progressLines is written to the progress writer, so a test can
	// assert what a watching client would have seen.
	progressLines string
	inFlight      []string
}

func (f *fakeDeployer) InFlight() []string { return f.inFlight }

func (f *fakeDeployer) Deploy(ctx context.Context, name, ttl string, progress io.Writer) (ActionResult, error) {
	f.calls = append(f.calls, name)
	f.ttls = append(f.ttls, ttl)
	f.sawCtx = ctx
	f.ctxErr = ctx.Err()
	f.deadline, f.hadDl = ctx.Deadline()
	if progress != nil && f.progressLines != "" {
		_, _ = io.WriteString(progress, f.progressLines)
	}
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

// Minutes of silence reads as broken: a reader cannot tell a long apply
// from a hung one, and the difference matters when the thing running is
// creating billable infrastructure.
func TestDeployStreamsItsProgressToWatchers(t *testing.T) {
	hub := NewHub()
	client := &Client{send: make(chan []byte, 256)}
	hub.Register(client)

	srv := NewServer(ServerConfig{
		Config: config.Default(), Deployments: &fakeDeployments{}, Hub: hub,
		Deployer: &fakeDeployer{
			result:        ActionResult{Clean: true},
			progressLines: "Deploying lb-serving-paris\n  workdir: /tmp/x\n",
		},
	})

	rec := postDeploy(t, srv, `{"scenario":"lb-serving-paris"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var lines []string
	var subjects []string
	for {
		select {
		case raw := <-client.send:
			var e struct {
				Type string         `json:"type"`
				Data map[string]any `json:"data"`
			}
			require.NoError(t, json.Unmarshal(raw, &e))
			// Progress lines are the ONLY thing on this stream.
			// There is no terminal event: S163e deleted it along with
			// the machinery that consumed it, because a tab that did
			// not issue the POST has no business inferring an outcome
			// from a broadcast -- the estate page reads the record.
			require.Equal(t, "deploy_progress", e.Type, "no other event kind belongs on this stream")
			// Comma-ok, not a bare assertion: a regression that drops
			// `subject` or nests it would panic the whole test binary
			// and report as a panic rather than as the assertion that
			// names the missing field.
			line, ok := e.Data["line"].(string)
			require.True(t, ok, "every progress event carries a line")
			subject, ok := e.Data["subject"].(string)
			require.True(t, ok, "every progress event names its subject")
			lines = append(lines, line)
			subjects = append(subjects, subject)
		default:
			// Indentation is PRESERVED. The deploy command indents
			// sub-steps on purpose, and flattening them here would
			// throw away structure the reader uses to see where they
			// are in a multi-minute apply.
			require.Equal(t, []string{"Deploying lb-serving-paris", "  workdir: /tmp/x"}, lines)
			for _, s := range subjects {
				assert.Equal(t, "lb-serving-paris", s,
					"a line arriving on a page the reader moved to must say what it is about")
			}
			return
		}
	}
}

// A deploy must not depend on anybody LISTENING: the hub exists but has
// no clients, which is the ordinary case.
//
// Note this does not exercise the sink's nil-hub branch -- NewServer
// substitutes a hub when ServerConfig.Hub is nil, so `state.hub` is
// never nil here. That branch is covered directly by
// TestProgressSinkWithNoHubIsHarmless.
func TestDeployWorksWithNobodyListening(t *testing.T) {
	srv := NewServer(ServerConfig{
		Config: config.Default(), Deployments: &fakeDeployments{},
		Deployer: &fakeDeployer{result: ActionResult{Clean: true}, progressLines: "working\n"},
	})

	rec := postDeploy(t, srv, `{"scenario":"lb-serving-paris"}`)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// The page cannot be the guard: a refresh wipes its state, a second tab
// never had it, and a `curl` never consulted it. Two deploys of one
// scenario are two run-owned projects and two sets of billable resources
// for one thing.
func TestASecondDeployOfTheSameScenarioIsRefused(t *testing.T) {
	srv := deployServer(t, &fakeDeployer{err: ErrDeployInProgress})

	rec := postDeploy(t, srv, `{"scenario":"web-app-paris"}`)

	// 423, not 409: 409 on this endpoint carries an ActionResult from a
	// deploy that RAN. A refusal that shares its status is parsed as
	// one, and the reader is told resources may be leaking after a
	// request that touched nothing.
	assert.Equal(t, http.StatusLocked, rec.Code, "reasonable request, unreasonable moment")
	assert.Contains(t, rec.Body.String(), "web-app-paris",
		"a bare refusal leaves a reader wondering which of their tabs is responsible")
	assert.NotContains(t, rec.Body.String(), `"clean"`,
		"a refusal is not an action result and must not be mistakable for one")
}

// An applying deploy has no record yet, so it cannot appear in
// `deployments` -- and a listing that showed only records would call an
// estate empty while it was busy creating one. The estate page renders
// this list as its own banner.
//
// Not for restoring a reloaded page: that consumer was deleted in
// S163e, and the field outlived it.
func TestTheListingNamesWhatIsCurrentlyDeploying(t *testing.T) {
	srv := NewServer(ServerConfig{
		Config: config.Default(), Deployments: &fakeDeployments{},
		Deployer: &fakeDeployer{inFlight: []string{"web-app-paris"}},
	})

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/deployments", nil))

	var payload deploymentsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.Equal(t, []string{"web-app-paris"}, payload.Deploying)
}

// A nil slice marshals as `null`, which a client reads as unknown or
// crashes on.
func TestTheListingReportsAnEmptyDeployingListRatherThanNull(t *testing.T) {
	for name, deployer := range map[string]DeploymentDeployer{
		"no deployer configured":       nil,
		"deployer with none in flight": &fakeDeployer{inFlight: nil},
	} {
		t.Run(name, func(t *testing.T) {
			srv := NewServer(ServerConfig{
				Config: config.Default(), Deployments: &fakeDeployments{}, Deployer: deployer,
			})
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/deployments", nil))

			assert.Contains(t, rec.Body.String(), `"deploying":[]`)
			assert.NotContains(t, rec.Body.String(), `"deploying":null`)
		})
	}
}

// responseTimingRecorder notes what had been broadcast by the time the
// response headers were written.
type responseTimingRecorder struct {
	*httptest.ResponseRecorder
	send            chan []byte
	broadcastFirst  bool
	wroteHeaderOnce bool
}

func (w *responseTimingRecorder) WriteHeader(code int) {
	if !w.wroteHeaderOnce {
		w.wroteHeaderOnce = true
		w.broadcastFirst = len(w.send) > 0
	}
	w.ResponseRecorder.WriteHeader(code)
}

// The trailing partial line has to be flushed BEFORE the response, and
// the ordering is a property of the client rather than of this handler.
//
// A page stops accepting progress for a deploy the moment its request
// resolves -- an entry that is no longer running must not absorb
// somebody else's stream. So a line emitted after the response is a line
// the browser discards: the tail of a billable apply's log, dropped
// silently, by the very flush that exists to guarantee it is not.
//
// A deferred Close alone put the flush after `writeActionResult`. It is
// still deferred, for panics; it is also called explicitly, and Close is
// idempotent so the two do not fight.
func TestTheLastLineOfAnApplyIsBroadcastBeforeTheResponse(t *testing.T) {
	hub := NewHub()
	client := &Client{send: make(chan []byte, 256)}
	hub.Register(client)

	srv := NewServer(ServerConfig{
		Config: config.Default(), Deployments: &fakeDeployments{}, Hub: hub,
		Deployer: &fakeDeployer{
			result: ActionResult{Clean: true},
			// No trailing newline: this is the line only Close emits.
			progressLines: "  destroy: done",
		},
	})

	rec := &responseTimingRecorder{ResponseRecorder: httptest.NewRecorder(), send: client.send}
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/deployments",
		strings.NewReader(`{"scenario":"lb-serving-paris"}`)))
	require.Equal(t, http.StatusOK, rec.Code)

	assert.True(t, rec.broadcastFirst,
		"a line emitted after the response is one the page has already stopped accepting")

	var e struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(<-client.send, &e))
	assert.Equal(t, "  destroy: done", e.Data["line"])
}
