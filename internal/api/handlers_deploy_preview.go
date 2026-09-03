package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/redscaresu/infrafactory/internal/livestore"
	"github.com/redscaresu/infrafactory/internal/scenario"
)

// deployPreview is what a person must read before deciding to spend
// money (ADR-0027 §2).
//
// Four things, and each is there because leaving it out has a specific
// failure mode:
//
//   - **what will be created**, so "deploy" is not an abstraction;
//   - **cost, at list price, admitting it is an estimate** -- a
//     confidently wrong number shown at the moment somebody decides to
//     spend is worse than an admitted one;
//   - **expiry as a WALL-CLOCK time**, because "4h" is a number people
//     agree to without doing the arithmetic;
//   - **whether it is reachable from the internet**, which is a
//     different question from what it costs and is the one people forget
//     to ask.
type deployPreview struct {
	Scenario string `json:"scenario"`
	Cloud    string `json:"cloud"`

	// Deployable is false when this scenario cannot be deployed at all,
	// and Reason says why. Reported rather than 4xx'd because a page
	// listing scenarios wants to explain the greyed-out ones.
	Deployable bool   `json:"deployable"`
	Reason     string `json:"reason,omitempty"`

	Image string `json:"image,omitempty"`

	TTL        string `json:"ttl,omitempty"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`

	// A POINTER. `omitempty` does not omit a zero `time.Time` -- it is a
	// struct, so the tag has nothing to act on -- and an undeployable
	// preview never sets this. Without the pointer every greyed-out
	// scenario would report expiring in the year 1, which a page renders
	// as a date and a reader trusts.
	//
	// The same defect S159a closed for the deployments payload. It came
	// back here because that class test asserted over ONE payload, and
	// this is a new one.
	ExpiresAt    *time.Time `json:"expires_at"`
	ExpiresLocal string     `json:"expires_at_wall_clock,omitempty"`

	Cost        scenario.CostEstimate `json:"cost"`
	CostSummary string                `json:"cost_summary,omitempty"`

	// InternetFacing is true when the shape includes a public address.
	InternetFacing bool `json:"internet_facing"`

	// AlreadyDeploying is true when this scenario is applying right now.
	//
	// Separate from AlreadyLive because it is a different fact with a
	// different consequence: that deploy has no record yet, and a second
	// attempt will be refused rather than duplicating anything.
	AlreadyDeploying bool `json:"already_deploying"`

	// AlreadyLiveUnknown is true when the estate could not be read.
	//
	// An empty AlreadyLive is a CLAIM -- "checked, and nothing exists" --
	// and a guard whose job is warning about existing billable
	// infrastructure must not make that claim when it failed to look.
	// The deployments listing already carries `unreadable` for exactly
	// this; this is the same idea at the preview.
	AlreadyLiveUnknown bool `json:"already_live_unknown"`

	// AlreadyLive names deployments of this scenario that are already
	// running.
	//
	// The in-flight lock stops the ACCIDENTAL duplicate -- the reload,
	// the second tab, the double click. It does nothing about the
	// deliberate one: deploy, wait for it to finish, deploy again, and
	// there are two run-owned projects and two sets of billable
	// resources for one scenario.
	//
	// That is not a lock's job. A lock cannot tell "I forgot" from "I
	// meant it", and refusing outright would break redeploying after a
	// teardown. So the confirmation says what already exists and the
	// reader decides -- which is the same shape as every other decision
	// on that screen.
	AlreadyLive []string `json:"already_live"`

	// Allowed reports whether this server would accept the deploy.
	//
	// Carried here rather than fetched separately because the preview is
	// meant to be everything a person needs in order to decide, and
	// "this server will refuse" is part of that. It stops the page
	// offering a button that 404s; the SAFETY is that the endpoint does
	// not exist, and this field cannot make it exist.
	Allowed bool `json:"deploy_allowed"`
}

// deployPreviewHandler answers what a deploy would do. It is a GET
// because it does nothing.
//
// Available whether or not `--allow-deploy` was given: knowing what a
// deployment would cost and expose is information, not a capability, and
// a page that can explain why a button is disabled is better than one
// that silently omits it.
func deployPreviewHandler(state *serverState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		name := r.URL.Query().Get("scenario")
		if name == "" {
			writeJSONError(w, http.StatusBadRequest, "scenario is required")
			return
		}

		relPath, err := findScenarioPathByName(state, name)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSONError(w, http.StatusNotFound, "scenario not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// findScenarioPathByName strips the extension, and it accepts
		// BOTH .yaml and .yml. Appending .yaml would 500 on every
		// .yml-backed scenario -- a file the discovery function
		// deliberately supports.
		sc, err := loadScenarioByRelPath(state, relPath, name)
		if err != nil {
			if os.IsNotExist(err) {
				writeJSONError(w, http.StatusNotFound, "scenario not found")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		preview := previewFor(&sc, r.URL.Query().Get("ttl"), time.Now())
		preview.Allowed = state.deployer != nil
		preview.AlreadyLive, preview.AlreadyLiveUnknown = liveDeploymentsOf(state, sc.Name)
		// A deploy that is APPLYING has no record yet -- registration
		// runs after the apply returns -- so the estate cannot see the
		// one case where the reader is most likely to be duplicating.
		// The in-flight list can.
		for _, deploying := range deployingScenarios(state) {
			if deploying == sc.Name {
				preview.AlreadyDeploying = true
			}
		}
		writeJSON(w, http.StatusOK, preview)
	}
}

// loadScenarioByRelPath reads a scenario from an extension-less relative
// path, trying the suffixes the discovery walk accepts.
//
// `want` is the name the caller asked for, and the loaded scenario must
// match it. Discovery matches on the scenario's NAME rather than its
// filename and returns an extension-less path, so `foo.yaml` and
// `foo.yml` holding different scenarios would let this load the wrong
// one. The consequence is not a 500 -- it is a confirmation dialog
// showing the cost, lifetime and blast radius of a DIFFERENT scenario,
// immediately before somebody agrees to spend money.
//
// Checking the name closes that generally rather than closing the one
// filename layout that exposed it.
func loadScenarioByRelPath(state *serverState, relPath, want string) (scenario.Scenario, error) {
	base := filepath.Join(state.cfg.Paths.Scenarios, filepath.FromSlash(relPath))

	candidates := []string{base}
	if filepath.Ext(base) == "" {
		candidates = []string{base + ".yaml", base + ".yml"}
	}

	var lastErr error = os.ErrNotExist
	for _, candidate := range candidates {
		sc, _, err := loadScenarioFile(candidate, state.scenarioSchemaPathCandidates())
		if err == nil {
			if want != "" && sc.Name != want {
				// Right path, wrong scenario. Keep looking rather than
				// answering about something the caller did not ask for.
				lastErr = os.ErrNotExist
				continue
			}
			return sc, nil
		}
		if !os.IsNotExist(err) {
			// A file that exists and will not parse is a real error and
			// must not be masked by trying the other suffix.
			return scenario.Scenario{}, err
		}
		lastErr = err
	}
	return scenario.Scenario{}, lastErr
}

// previewFor assembles the preview, and is separate from the handler so
// it can be tested without HTTP.
func previewFor(sc *scenario.Scenario, ttlOverride string, now time.Time) deployPreview {
	preview := deployPreview{Scenario: sc.Name, Cloud: sc.Cloud}

	// The cost and shape are worth knowing even for a scenario that
	// cannot be deployed -- that is exactly what explains the greyed-out
	// button.
	preview.Cost = sc.EstimateCost()
	// Derived from the estimate, not re-decided from the scenario.
	//
	// These were two hand-written conditions and they disagreed: the
	// estimator bills a public IPv4 for every compute instance, while
	// this checked only the load balancer -- so a compute-only scenario
	// was charged for a public address and told it was not reachable
	// from the internet. Understating exposure at the moment of the
	// decision is the worse direction of the two.
	preview.InternetFacing = preview.Cost.InternetFacing()

	if sc.Service == nil {
		// The same refusal `runDeployCommand` makes: without a versioned
		// application there is nothing whose version could be rolled
		// forward, and "deploy" would just mean "apply and forget to
		// destroy".
		preview.Reason = "this scenario declares no service: block, so there is nothing to deploy. " +
			"Use a run to validate infrastructure-only scenarios"
		return preview
	}

	preview.Image = sc.Service.Ref()

	spec := *sc.Service
	if ttlOverride != "" {
		spec.TTL = ttlOverride
		if err := spec.Validate(); err != nil {
			preview.Reason = fmt.Sprintf("ttl: %v", err)
			return preview
		}
	}

	ttl, err := spec.TimeToLive()
	if err != nil {
		// ADR-0024 has no unbounded form, so a TTL that will not parse
		// makes the scenario undeployable rather than defaulting to
		// something. Guessing a lifetime is how a deployment outlives
		// everyone's memory of it.
		preview.Reason = fmt.Sprintf("ttl: %v", err)
		return preview
	}

	preview.Deployable = true
	preview.TTL = ttl.String()
	preview.TTLSeconds = int64(ttl.Seconds())
	expires := now.Add(ttl)
	preview.ExpiresAt = &expires
	// Wall clock, spelled out. "4h" is a number people agree to without
	// doing the arithmetic; "expires at 03:47" is one they check against
	// whether they will still be awake.
	preview.ExpiresLocal = expires.Format("Mon 2 Jan 15:04 MST")
	preview.CostSummary = preview.Cost.Summary(ttl)

	return preview
}

// liveDeploymentsOf names the deployments of a scenario that have not
// been released, and reports whether the answer is complete.
//
// The second return is not a detail: an empty list is a CLAIM -- checked,
// and nothing exists -- and a guard whose job is warning about existing
// billable infrastructure must not make it without having looked. Every
// path that did not look says so.
func liveDeploymentsOf(state *serverState, name string) ([]string, bool) {
	out := []string{}
	if name == "" {
		// Never looked, so no claim. A scenario whose `scenario:` key is
		// blank still resolves to a file, and the confirmation would
		// otherwise render with no warning and no caveat.
		return out, true
	}
	if state.deployments == nil {
		// No lister, so nothing was checked. Returning (empty, false)
		// would be the claim this flag exists to forbid -- and the
		// err != nil branch below gets it right, which is the whole
		// argument.
		return out, true
	}

	deployments, unreadable, err := state.deployments.List()
	if err != nil {
		// Could not look. Saying "nothing exists" here would be a
		// false negative on a guard about billable infrastructure.
		return out, true
	}

	for _, d := range deployments {
		if d.Scenario == name && d.State != livestore.StateReleased {
			out = append(out, d.ID)
		}
	}
	sort.Strings(out)

	// A record that will not decode has no Scenario to match on, so it
	// could be a deployment of THIS scenario and would never appear
	// above. That is a gap in the answer, not a complete one.
	return out, len(unreadable) > 0
}
