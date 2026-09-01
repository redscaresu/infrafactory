package livestore

import "sort"

// PromotionRule decides when an observation stops being an incident and
// becomes a candidate lesson (S156b).
//
// The question this slice exists to answer is "when is an observation a
// lesson?", and the answer is **when it has reproduced** — not when it is
// severe, and not when Terraform also failed. A terraform failure is
// already learned from by the run loop; live observation exists for the
// failures Terraform reports as success.
type PromotionRule struct {
	// ConsecutiveProbes is how many probes in a row must report the same
	// thing on ONE deployment before it counts as persistent rather than
	// a blip.
	ConsecutiveProbes int

	// DistinctDeployments is how many separate deployments must report
	// the same thing before it counts as a property of the shape rather
	// than of one machine.
	DistinctDeployments int

	// Normalize collapses cosmetic variation — line numbers, instance
	// suffixes, provider phrasing — so the same underlying failure
	// groups with itself.
	//
	// Injected rather than imported so this package keeps its
	// deliberately small dependency set; `feedback.NormalizeDetail` is
	// what callers pass.
	Normalize func(string) string
}

// DefaultPromotionRule is the starting threshold.
//
// Both numbers are guesses and are stated as such. There is no corpus of
// live observations to tune them against, and the plan is explicit that
// thresholds live in configuration so they can be raised when a real one
// arrives. Three consecutive probes is enough to outlast a restart; two
// deployments is the smallest number that can distinguish a property of
// the shape from a property of one machine.
var DefaultPromotionRule = PromotionRule{
	ConsecutiveProbes:   3,
	DistinctDeployments: 2,
}

// Candidate is one reproduced observation, with the evidence for it.
//
// It is deliberately not a pitfall. Turning it into one is S156c's job,
// and keeping the two apart means the gate can be judged on whether it
// promotes the right things without also arguing about rule text.
type Candidate struct {
	// Status distinguishes "it told us it is broken" from "we got no
	// answer" — different facts that must not merge (ADR-0024, S154).
	Status ObservationStatus

	// Detail is the normalized form, and the identity of the candidate.
	Detail string

	// Example is one raw detail exactly as observed. The normalized form
	// is for grouping; a human, and eventually a rule, needs the real
	// words.
	Example string

	// Scenarios are the scenarios that exhibited it, sorted. More than
	// one is strong evidence of a shape-level problem.
	Scenarios []string

	// Deployments are the deployment ids that exhibited it, sorted.
	Deployments []string

	// LongestRun is the most consecutive probes on a single deployment.
	LongestRun int

	// Attributable reports whether at least one exhibiting deployment
	// had its running version CONFIRMED at the time.
	//
	// Not a filter, deliberately. An unattributable observation is still
	// real — something was broken — but a lesson blamed on a version
	// nobody verified is a falsehood (S155a), so the fact travels with
	// the candidate and the extractor decides what to do with it.
	Attributable bool

	// Reason records which half of the rule promoted it, because
	// "persistent on one deployment" and "seen on several" are different
	// kinds of evidence and a reader should not have to infer which.
	Reason PromotionReason
}

// PromotionReason is why a candidate cleared the gate.
type PromotionReason string

const (
	// PromotedByPersistence means one deployment reported it enough
	// times in a row.
	PromotedByPersistence PromotionReason = "persistent"
	// PromotedByBreadth means separate deployments reported it.
	PromotedByBreadth PromotionReason = "reproduced across deployments"
	// PromotedByBoth is the strongest evidence available here.
	PromotedByBoth PromotionReason = "persistent and reproduced across deployments"
)

// PromotionCandidates returns the observations that have reproduced.
//
// Three rules worth stating, because each one is a way the gate could be
// wrong rather than merely incomplete:
//
//   - **Healthy observations are not candidates.** They carry no detail;
//     there is nothing to learn from a service that worked.
//   - **A run is broken by anything that is not the same failure.** A
//     healthy probe between two 503s means the service recovered, which
//     is precisely the blip this gate exists to reject. Counting them as
//     consecutive would promote a flapping service as a structural fact.
//   - **Released deployments still count.** Their observations happened
//     while they were live, and the record keeps its history on purpose.
//     Dropping them would discard exactly the reproduced evidence the
//     gate is looking for.
func PromotionCandidates(deployments []Deployment, rule PromotionRule) []Candidate {
	normalize := rule.Normalize
	if normalize == nil {
		normalize = func(s string) string { return s }
	}

	type key struct {
		status ObservationStatus
		detail string
	}
	type evidence struct {
		example      string
		scenarios    map[string]bool
		deployments  map[string]bool
		longestRun   int
		attributable bool
	}

	seen := map[key]*evidence{}

	for _, d := range deployments {
		// Per deployment, walk the observations in order so a run is a
		// run in time rather than a count.
		runs := map[key]int{}
		for _, o := range d.Observations {
			if o.Healthy() || o.Detail == "" {
				// Anything that is not a failure ends every run: a
				// service that recovered did not keep failing.
				runs = map[key]int{}
				continue
			}

			k := key{status: o.Status, detail: normalize(o.Detail)}

			// This observation continues its own run and breaks every
			// other, because only one thing can be true of a service at
			// a given probe.
			current := runs[k] + 1
			runs = map[key]int{k: current}

			e := seen[k]
			if e == nil {
				e = &evidence{
					example:     o.Detail,
					scenarios:   map[string]bool{},
					deployments: map[string]bool{},
				}
				seen[k] = e
			}
			if d.Scenario != "" {
				e.scenarios[d.Scenario] = true
			}
			e.deployments[d.ID] = true
			if current > e.longestRun {
				e.longestRun = current
			}
			if o.Version == VersionConfirmed {
				e.attributable = true
			}
		}
	}

	var out []Candidate
	for k, e := range seen {
		persistent := rule.ConsecutiveProbes > 0 && e.longestRun >= rule.ConsecutiveProbes
		broad := rule.DistinctDeployments > 0 && len(e.deployments) >= rule.DistinctDeployments
		if !persistent && !broad {
			continue
		}

		out = append(out, Candidate{
			Status:       k.status,
			Detail:       k.detail,
			Example:      e.example,
			Scenarios:    sortedKeys(e.scenarios),
			Deployments:  sortedKeys(e.deployments),
			LongestRun:   e.longestRun,
			Attributable: e.attributable,
			Reason:       promotionReason(persistent, broad),
		})
	}

	// Strongest evidence first: more deployments, then longer runs, then
	// detail for a stable order.
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Deployments) != len(out[j].Deployments) {
			return len(out[i].Deployments) > len(out[j].Deployments)
		}
		if out[i].LongestRun != out[j].LongestRun {
			return out[i].LongestRun > out[j].LongestRun
		}
		return out[i].Detail < out[j].Detail
	})
	return out
}

func promotionReason(persistent, broad bool) PromotionReason {
	switch {
	case persistent && broad:
		return PromotedByBoth
	case broad:
		return PromotedByBreadth
	default:
		return PromotedByPersistence
	}
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
