package livestore

import "time"

// HealthUnobserved is the state of a deployment nobody has probed.
//
// Deliberately NOT an ObservationStatus: it is the absence of one, and
// giving it the same type would let it be compared with, defaulted to, or
// mistaken for a probe result. It exists because the alternative -- an
// empty string, or a blank cell -- reads as "fine", and rebuilding that
// falsehood is what the three-state design exists to prevent.
const HealthUnobserved = "unobserved"

// HealthSummary is what a reader needs to know about whether a live
// deployment is actually serving.
//
// It lives here, on the record, rather than being assembled by each
// caller. The CLI's `live ls` and the API's deployments listing must
// agree about what silence means, and two implementations of "take the
// last observation, unless there are none" is how one of them eventually
// renders `unobserved` as healthy -- silently, and only in the UI, where
// it is least likely to be noticed.
type HealthSummary struct {
	// Status is a probe status, or HealthUnobserved when there is none.
	Status string `json:"status"`

	// Version is `confirmed`, `unconfirmed`, or `unchecked`, ALWAYS
	// spelled out.
	//
	// A string rather than a VersionCheck, and that is the whole point:
	// `VersionUnchecked` is the empty string on the stored record, which
	// is fine there -- `omitempty` keeps the file terse -- and is a
	// falsehood in a view. A UI rendering `""` shows a blank cell beside
	// a `confirmed` one and invites the reader to read nothing-was-checked
	// as nothing-is-wrong. This is a view, so it says the word.
	//
	// Carried separately from Status because they answer different
	// questions, and the dangerous state is where they disagree: a
	// service answering perfectly while running something other than
	// what the record claims looks healthy to every other signal.
	Version string `json:"version"`

	// At is when the deployment was last observed, or nil when never.
	//
	// A POINTER, because `omitempty` does not omit a zero time.Time --
	// it is a struct, so the tag has no effect and it marshals as
	// `0001-01-01T00:00:00Z`. A page would then show a never-probed
	// deployment as last observed in the year 1: worse than a blank
	// cell, because it looks like data.
	//
	// The same defect the Version field above exists to avoid, one line
	// down and in a different disguise.
	At *time.Time `json:"at"`

	// Detail is the reason, when there is one.
	Detail string `json:"detail,omitempty"`

	// Observations is how many probes this record holds, so a reader can
	// tell one lucky sample from a settled picture.
	Observations int `json:"observations"`
}

// Observed reports whether anybody has ever probed this deployment.
func (h HealthSummary) Observed() bool { return h.Status != HealthUnobserved }

// Health summarises the most recent observation.
func (d Deployment) Health() HealthSummary {
	if len(d.Observations) == 0 {
		// Nobody looked, so nothing is known about the running version
		// either.
		return HealthSummary{Status: HealthUnobserved, Version: VersionLabelUnchecked}
	}

	last := d.Observations[len(d.Observations)-1]
	at := last.At
	return HealthSummary{
		Status:       string(last.Status),
		Version:      versionLabel(last.Version),
		At:           &at,
		Detail:       last.Detail,
		Observations: len(d.Observations),
	}
}

// VersionLabelUnchecked is what `VersionUnchecked` is called in a view.
const VersionLabelUnchecked = "unchecked"

// versionLabel spells out a VersionCheck, including the empty one.
func versionLabel(v VersionCheck) string {
	if v == VersionUnchecked {
		return VersionLabelUnchecked
	}
	return string(v)
}
