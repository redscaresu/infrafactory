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

	// Deployment names the live record, when one was written.
	//
	// A failed deploy is not the same as an unrecorded one. `deploy`
	// registers from whatever the state shows, whether or not the apply
	// succeeded, so a half-failed apply usually DOES leave a record --
	// with a TTL, listed on the estate page, reapable. Without this the
	// client had to assume the worst of every unclean deploy and pin a
	// permanent "it may have created resources that are still running"
	// alarm for infrastructure the estate already tracks, unable even
	// to name what to tear down.
	//
	// Empty means no record: either nothing was created, or something
	// was and could not be recorded. Those are told apart by the
	// failures, which say which.
	Deployment string `json:"deployment,omitempty"`
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

// ErrNothingStarted marks a Deploy failure that happened BEFORE
// anything touched the cloud.
//
// The claim is about infrastructure, not about blame: it says no
// project was created and no resources exist, so a client can say "that
// request did nothing" instead of "it may have created resources that
// are still running". Nothing else can tell -- a deploy is detached
// from the request that starts it, and every failure mode looks the
// same from outside.
//
// Only the deployer knows. It resolves the name, rebuilds its runtime
// and parses its flags before the apply begins, and each of those can
// fail; without a way to say so, every one of them pinned a permanent
// false leak report for a request that never reached Scaleway.
var ErrNothingStarted = errors.New("nothing was started")

// ErrNoSuchScenario means the name did not resolve.
//
// A refinement of ErrNothingStarted rather than a separate fact -- it
// answers 404 where its parent answers 500 -- and both promise the same
// thing about the cloud. Distinct from a bare `os.ErrNotExist`, which
// says a file was missing and says nothing about WHEN: a state file or
// a workdir vanishing mid-apply produces one of those too.
var ErrNoSuchScenario error = nothingStartedSentinel{msg: "no such scenario"}

// nothingStartedSentinel is a sentinel that refines ErrNothingStarted
// WITHOUT concatenating its text.
//
// `fmt.Errorf("no such scenario: %w", ErrNothingStarted)` gives the
// sentinel an Error() of "no such scenario: nothing was started" -- and
// the handler writes err.Error() straight into the refusal body for a
// page to render. `LiveDeployer` escapes it only because it returns its
// own error type; the interface is exported, and an implementer that
// signals the documented way (`return api.ErrNoSuchScenario`) would put
// a self-contradicting sentence in front of an operator.
//
// Same rule the CLI's own wrappers follow: a sentinel is for
// `errors.Is`; a message is for a person.
type nothingStartedSentinel struct{ msg string }

func (e nothingStartedSentinel) Error() string { return e.msg }

func (e nothingStartedSentinel) Is(target error) bool { return target == ErrNothingStarted }

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
