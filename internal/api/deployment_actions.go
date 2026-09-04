package api

import (
	"context"
	"errors"
	"io"
)

// ActionStep is one thing an action did, in the shape a page can render.
//
// Deliberately not the CLI's StageSummary: this package does not import
// `internal/cli`, and a neutral shape at the seam is what keeps the
// dependency pointing one way -- the same arrangement RunStarter uses.
type ActionStep struct {
	Stage  string `json:"stage"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// ActionResult is what happened, reported whether or not it worked.
//
// `Clean` is a separate field from the absence of failures because they
// are different claims. ADR-0024's rule is that a teardown which cannot
// PROVE the account is clean must not report success -- a deployment
// whose state has vanished is not released, because marking it released
// would retire the only record saying its resources may still exist.
type ActionResult struct {
	Clean bool         `json:"clean"`
	Steps []ActionStep `json:"steps"`

	// Failures are the reasons it is not clean. Carried separately so a
	// page cannot render a partial success as a success.
	Failures []ActionStep `json:"failures"`
}

// DeploymentDeployer creates a live deployment.
//
// Separate from DeploymentActor, and that separation is ADR-0027's whole
// argument in the type system: destroying what exists and creating what
// persists are different kinds of harm, gated by different flags. A
// server holding one of these interfaces cannot be talked into the
// other.
//
// # Streaming
//
// Deploy takes an `io.Writer` for progress. A deploy runs for minutes,
// and minutes of silence reads as broken -- the reader cannot tell a
// long apply from a hung one, and the difference matters when the thing
// running is creating billable infrastructure.
//
// The writer is supplied by the caller rather than the deployer reaching
// for a hub, so this package keeps deciding what goes on the wire and
// `internal/cli` keeps not knowing there is one.
type DeploymentDeployer interface {
	// Deploy applies a scenario and leaves it running under a TTL.
	//
	// It takes the scenario NAME and a TTL string, and nothing else. It
	// deliberately cannot be told which project to use -- run-owned
	// projects are created by the harness (ADR-0025), and a request that
	// could name one is a request that could name somebody else's.
	Deploy(ctx context.Context, scenarioName, ttl string, progress io.Writer) (ActionResult, error)

	// InFlight names the scenarios currently deploying.
	//
	// An applying deploy has no record until registration, which runs
	// after the apply returns -- so it is invisible to any listing of
	// records, and an estate busy creating something would be described
	// as empty. This is what the estate page renders instead, and what
	// the preview warns on.
	//
	// The list is advisory and load-bearing nowhere: the refusal is the
	// lock inside Deploy, so a client that ignores this cannot start
	// two. It is also NOT a complete answer to "what is applying" -- it
	// is one process's memory, so a CLI deploy, or an apply in flight
	// when the server restarted, is absent from it.
	//
	// (This previously said it existed so a reloaded page could restore
	// what it was showing, and that the server had no lock. The first
	// consumer was deleted in S163e; the second was false as of S163c.
	// Corrected 2026-09-03.)
	InFlight() []string
}

// ErrDeployInProgress is returned when a scenario is already deploying.
//
// Per scenario rather than globally: two different scenarios deploying
// at once is ordinary, and blocking it would make the UI worse for no
// safety gain. Two deploys of the SAME scenario produce two run-owned
// projects and two sets of billable resources for one thing, which is
// never what was meant.
var ErrDeployInProgress = errors.New("a deploy of this scenario is already running")

// ErrNoSuchScenario means the name did not resolve, BEFORE anything ran.
//
// Distinct from a bare `os.ErrNotExist`, and the distinction is the
// whole point. Both answer 404, but only this one can promise nothing
// was created -- and that promise is what the client uses to decide
// whether to keep a "may have created resources that are still running"
// report pinned on screen.
//
// Without it, a typo'd scenario name pinned exactly that report for the
// rest of the session, for a scenario that never existed: the handler
// could not tell a name that failed to resolve before the apply from an
// os.ErrNotExist surfacing out of one that had already begun.
var ErrNoSuchScenario = errors.New("no such scenario")

// DeploymentActor performs the destructive half of live management.
//
// Separate from DeploymentLister on purpose. Reading the estate is safe
// and always available; destroying infrastructure is neither, and a
// server that can only read cannot be talked into more than reading
// (S159b).
type DeploymentActor interface {
	// Teardown destroys one deployment's infrastructure.
	Teardown(ctx context.Context, id string) (ActionResult, error)

	// Reap destroys every deployment whose TTL has run out.
	Reap(ctx context.Context) (ActionResult, error)
}
