package api

import (
	"net/http"
	"sort"
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
			Schema:      "infrafactory.api.deployments.v1",
			Deployments: make([]deploymentJSON, 0, len(deployments)),
			Unreadable:  make([]string, 0, len(unreadable)),
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
