package api

import (
	"context"
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
}

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
