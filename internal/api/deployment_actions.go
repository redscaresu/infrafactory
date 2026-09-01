package api

import "context"

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
