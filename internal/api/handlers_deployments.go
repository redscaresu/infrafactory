package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/redscaresu/infrafactory/internal/livestore"
)

// DeploymentLister reads the live estate.
//
// An interface rather than a concrete store because the API package does
// not own where the estate lives -- `internal/cli` resolves the live
// store root from configuration and flags -- and the same seam already
// carries RunStarter.
type DeploymentLister interface {
	List() ([]livestore.Deployment, []error, error)
}

// deploymentJSON is one deployment as the UI needs it.
//
// The health summary comes from `Deployment.Health()` rather than being
// assembled here, so this listing and `live ls` cannot disagree about
// what silence means (S159a).
type deploymentJSON struct {
	livestore.Deployment

	// Unreadable is re-exported because the embedded field is
	// `json:"-"`. Without it a consumer sees a phantom deployment with
	// no scenario, project or state and no way to tell it from a real
	// record -- the same reason `live ls` re-exports it.
	Unreadable bool                    `json:"unreadable"`
	Health     livestore.HealthSummary `json:"health"`
	Expired    bool                    `json:"expired"`
	TTLSeconds int64                   `json:"time_to_live_seconds"`
	Upgraded   bool                    `json:"upgraded"`

	// These SHADOW the embedded record's fields of the same name, for a
	// reason that is a property of the language rather than of this
	// record: `omitempty` does not omit a zero `time.Time`. It is a
	// struct, so the tag has nothing to act on and the field marshals
	// as `0001-01-01T00:00:00Z`.
	//
	// A deployment that was never upgraded would arrive carrying an
	// upgrade date in the year 1 -- worse than a missing field, because
	// a page renders it and a reader trusts a date.
	//
	// Every optional time in a view has this defect by default, which is
	// why TestDeploymentPayloadNeverCarriesAYearOneTimestamp checks the
	// whole payload rather than these two fields: the next optional time
	// added to the record inherits it silently.
	UpgradedAt       *time.Time `json:"upgraded_at"`
	UpgradeStartedAt *time.Time `json:"upgrade_started_at"`
}

// optionalTime is nil for the zero time, so an absent moment serialises
// as `null` rather than as the year 1.
func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

type deploymentsResponse struct {
	Schema      string           `json:"schema"`
	Deployments []deploymentJSON `json:"deployments"`

	// Unreadable carries records the store could not decode.
	//
	// Reported rather than omitted, and the count is what the UI must
	// show: a record that will not decode may describe running,
	// billing infrastructure, so a listing that silently dropped it
	// would make "we could not check" look like "nothing is running".
	// `live ls` exits non-zero for this; a GET cannot, so the payload
	// has to carry it where a page will see it.
	Unreadable []string `json:"unreadable"`

	// DeployAllowed reports whether this server was started with
	// --allow-deploy. Same purpose and same non-guarantee as
	// TeardownAllowed: it stops the page offering a button that 404s,
	// and it cannot make the endpoint exist.
	DeployAllowed bool `json:"deploy_allowed"`

	// Deploying names the scenarios currently applying.
	//
	// An applying deploy has no record yet, so it is absent from
	// `deployments` while being the most active thing in the estate; the
	// page renders this as its own banner so an estate busy creating
	// something is never described as empty.
	//
	// What it is NOT: a complete answer to "what is running". It is one
	// process's in-memory lock, so a CLI deploy or one that was in
	// flight across a restart is missing from it. The guard against a
	// second deploy is server-side and does not depend on this field.
	Deploying []string `json:"deploying"`

	// TeardownAllowed reports whether this server was started with
	// --allow-teardown.
	//
	// Carried so a page does not offer a button it knows will 404. That
	// is a UI nicety; the SAFETY is that the endpoint does not exist,
	// and this field cannot make it exist. A client that sets it locally
	// gets a button that 404s.
	TeardownAllowed bool `json:"teardown_allowed"`
}

// deploymentsHandler serves the live estate, read-only.
//
// Deliberately read-only. Deploying, tearing down and reaping all carry
// guards that live in `internal/cli` and are not reachable from here
// without a seam that does not exist yet; shipping the read first lets
// the estate page be built against something real without any of those
// guards being reimplemented in a hurry.
func deploymentsHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// Creating shares the collection URL with listing, as REST
			// expects. The capability gate lives in deployHandler, so a
			// server without --allow-deploy answers 404 here rather than
			// 405 -- "no such thing" rather than "wrong verb".
			deployHandler(state)(w, r)
			return
		}
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if state.deployments == nil {
			writeJSONError(w, http.StatusNotImplemented, "live deployments are not configured")
			return
		}

		deployments, unreadable, err := state.deployments.List()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		now := time.Now()
		payload := deploymentsResponse{
			Schema:          "infrafactory.api.deployments.v1",
			Deployments:     make([]deploymentJSON, 0, len(deployments)),
			Unreadable:      make([]string, 0, len(unreadable)),
			TeardownAllowed: state.deploymentActor != nil,
			DeployAllowed:   state.deployer != nil,
			Deploying:       deployingScenarios(state),
		}
		for _, d := range deployments {
			payload.Deployments = append(payload.Deployments, deploymentJSON{
				Deployment: d,
				Unreadable: d.Undecodable,
				Health:     d.Health(),
				Expired:    d.Expired(now),
				TTLSeconds: int64(d.TimeToLive(now).Seconds()),
				Upgraded:   !d.UpgradedAt.IsZero(),

				UpgradedAt:       optionalTime(d.UpgradedAt),
				UpgradeStartedAt: optionalTime(d.UpgradeStartedAt),
			})
		}
		for _, e := range unreadable {
			payload.Unreadable = append(payload.Unreadable, e.Error())
		}

		// Soonest to expire first: the estate page's job is to show what
		// needs attention, and what is about to vanish needs it most.
		sort.SliceStable(payload.Deployments, func(i, j int) bool {
			return payload.Deployments[i].TTLSeconds < payload.Deployments[j].TTLSeconds
		})

		writeJSON(w, http.StatusOK, payload)
	}
}

// deploymentActionHandler serves the destructive half of live
// management: `DELETE /api/deployments/{id}` and
// `POST /api/deployments/reap`.
//
// # Why it can be absent
//
// A nil actor is a 404, not a 501, and the difference is deliberate. The
// server is started with `--allow-teardown` or it is not, and when it is
// not there is genuinely no such endpoint -- announcing "not
// implemented" would advertise a capability the operator declined.
//
// The gate is start-time for the same reason S160b moved real-cloud
// apply out of the request body: a REQUEST must not be able to talk this
// server into destroying infrastructure. The origin guard (S160a) stops
// a page the server did not serve from reaching here at all, and this
// stops the capability existing unless a person asked for it in the
// shell. Two properties, and the second survives a bug in the first.
//
// Teardown is not "safe because it only removes things". It is
// irreversible, it acts on real infrastructure, and a demo torn down
// mid-talk is a real cost even though the bill goes down.
func deploymentActionHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if state.deploymentActor == nil {
			writeJSONError(w, http.StatusNotFound,
				"this server was not started with --allow-teardown, so it cannot destroy deployments")
			return
		}

		tail := strings.TrimPrefix(r.URL.Path, "/api/deployments/")
		if tail == "reap" {
			if r.Method != http.MethodPost {
				writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			ctx, cancel := destructiveContext(r)
			defer cancel()
			result, err := state.deploymentActor.Reap(ctx)
			writeActionResult(w, result, err)
			return
		}

		if r.Method != http.MethodDelete {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		// One segment, and no separators. A deployment id addresses a
		// file in the live store, so anything that could climb out of it
		// is refused rather than cleaned up: `livestore.validateID`
		// guards the store itself, and this refuses before the store is
		// asked, so a traversal attempt is never a lookup.
		if tail == "" || strings.ContainsAny(tail, "/\\.") {
			writeJSONError(w, http.StatusBadRequest, "invalid deployment id")
			return
		}

		ctx, cancel := destructiveContext(r)
		defer cancel()

		result, err := state.deploymentActor.Teardown(ctx, tail)
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "no such deployment")
			return
		}
		writeActionResult(w, result, err)
	}
}

// writeActionResult reports what happened, and refuses to call a
// partial teardown a success.
//
// A result carrying failures is 409, not 200: ADR-0024's rule is that a
// teardown which cannot PROVE the account clean must not report success,
// and a page rendering a green tick over "the state file has vanished
// and the resources may still be running" is exactly the false green
// this project exists to avoid.
func writeActionResult(w http.ResponseWriter, result ActionResult, err error) {
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := http.StatusOK
	if !result.Clean {
		status = http.StatusConflict
	}
	writeJSON(w, status, result)
}

// destructiveTimeout bounds an action that is deliberately not
// cancellable by its caller.
//
// Generous rather than tight: a destroy plus an orphan sweep plus the
// project delete has taken minutes against real Scaleway, and cutting
// one short is the failure this timeout exists to avoid, not the one it
// exists to cause. It is a backstop against a hung provider call, not a
// deadline.
const destructiveTimeout = 30 * time.Minute

// destructiveContext detaches an action from the HTTP request.
//
// `r.Context()` is cancelled when the client disconnects -- closing the
// tab, navigating away, a flaky wifi hop. A teardown cancelled halfway
// has already deleted some resources and not others, and the live record
// then describes neither the old state nor the new one.
//
// The same rule `ensureRunProject` applies to creating a run's project:
// once an operation begins changing real infrastructure, the caller
// going away must not stop it. Whoever asked can leave; the destroy
// finishes and the record ends up describing what is actually there.
//
// The request's VALUES are kept (tracing, deadlines set by middleware
// upstream) while its cancellation is dropped.
func destructiveContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), destructiveTimeout)
}

// deployRequest is everything a caller may decide.
//
// Two fields, and the absences are the design. There is no project: a
// request that could name one could name somebody else's, and run-owned
// projects are the harness's to create (ADR-0025, ADR-0027). There is no
// "skip validation", no image override, and no way to ask for an
// unbounded lifetime.
type deployRequest struct {
	Scenario string `json:"scenario"`

	// TTL overrides the scenario's own. Empty means the scenario's,
	// which the schema already bounds -- there is deliberately no value
	// meaning "forever".
	TTL string `json:"ttl,omitempty"`
}

// deployHandler creates a live deployment.
//
// Absent unless the server was started with `--allow-deploy`, which is
// implied by neither `--allow-layer3` nor `--allow-teardown`: an
// ephemeral apply the run destroys, destroying what exists, and creating
// what persists are three different kinds of harm (ADR-0027).
func deployHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if state.deployer == nil {
			writeJSONError(w, http.StatusNotFound,
				"this server was not started with --allow-deploy, so it cannot create deployments")
			return
		}
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req deployRequest
		if r.Body != nil {
			defer r.Body.Close()
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid json body")
				return
			}
		}
		if strings.TrimSpace(req.Scenario) == "" {
			writeJSONError(w, http.StatusBadRequest, "scenario is required")
			return
		}

		// Detached, exactly as teardown is. An apply takes minutes and
		// creates infrastructure as it goes; a client disconnecting
		// halfway would leave resources with no completed record of what
		// was made -- the leak D6 taught this project to fear, arriving
		// by a different route (ADR-0027).
		ctx, cancel := destructiveContext(r)
		defer cancel()

		// Named with the scenario, so a line arriving on a page the
		// reader has since navigated to says what it is about -- the
		// rule S162c cost seven findings to learn.
		progress := NewProgressSink(state.hub, "deploy_progress", req.Scenario)
		// Deferred so a panic inside Deploy still flushes the buffered
		// trailing line. Close is idempotent -- it resets the buffer --
		// so the explicit call below does not make this one redundant;
		// they cover different exits.
		defer progress.Close()

		result, err := state.deployer.Deploy(ctx, req.Scenario, req.TTL, progress)

		// Flushed BEFORE the response.
		//
		// The page stops accepting progress for a deploy the moment its
		// request resolves, because an entry that is no longer running
		// must not absorb somebody else's stream. So a line emitted
		// after the response is a line the browser discards: the tail of
		// a billable apply's log, dropped silently, by the flush that
		// exists to guarantee it is never dropped.
		//
		// This makes the tail arrive first; it does NOT guarantee it.
		// The broadcast goes out on the websocket and the response on
		// the HTTP connection, and two connections have no ordering
		// between them -- an in-process test can only pin the order the
		// server writes in, which is what
		// TestTheLastLineOfAnApplyIsBroadcastBeforeTheResponse does.
		//
		// Left as the better of two orderings rather than solved,
		// because the residual is narrow: `deploy` terminates every line
		// it writes, so there is no tail to lose for today's producer
		// (see ProgressSink.Close). A producer that emits a partial
		// final line would need the line carried in the response body
		// instead of raced against it.
		progress.Close()

		if errors.Is(err, ErrDeployInProgress) {
			// 423 Locked, NOT 409.
			//
			// 409 is already taken on this endpoint by
			// `writeActionResult`, for a deploy that RAN and could not
			// prove itself clean -- and that response carries an
			// ActionResult. Two 409s with incompatible bodies and no
			// discriminator meant a client parsing the refusal as an
			// ActionResult found no `clean` field and told the reader
			// "resources may still be running" for a request that never
			// touched the cloud.
			//
			// Saying infrastructure might be leaking when nothing
			// happened is the most alarming way to be wrong here, so
			// the two cases get two statuses.
			//
			// Naming the scenario matters: a bare refusal leaves a
			// reader wondering which of their tabs is responsible.
			writeJSONError(w, http.StatusLocked,
				fmt.Sprintf("%s is already deploying; wait for it to finish or tear it down", req.Scenario))
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			// The caller named something that is not here. A client
			// typo or a stale scenario list is not a server fault, and
			// answering 500 teaches operators that 500 means nothing in
			// particular. Matches the teardown handler.
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}

		writeActionResult(w, result, err)
	}
}

// deployingScenarios asks the deployer what is in flight, and always
// answers with a list rather than nil.
//
// A nil slice marshals as `null`, which a client reads as "unknown" or
// crashes on. An empty estate and an unconfigured deployer are both
// "nothing is deploying" from a reader's point of view.
func deployingScenarios(state *serverState) []string {
	if state.deployer == nil {
		return []string{}
	}
	if inFlight := state.deployer.InFlight(); inFlight != nil {
		return inFlight
	}
	return []string{}
}
