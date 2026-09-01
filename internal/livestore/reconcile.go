package livestore

import "sort"

// Reconciliation is the difference between what the cloud holds and what
// this store believes (S157a).
//
// ADR-0024 promised this and S153 did not deliver it: the reaper trusts
// the store to know every deployment. `.infrafactory/live` sits INSIDE
// the working directory, so wiping the directory, switching branches, or
// running from a fresh clone loses the records while the load balancer,
// the instance and the public IPv4s keep running -- with a TTL nobody
// will ever enforce.
//
// Every other signal reports clean, because every other signal reads this
// store. That is the shape of D6: a leak whose only symptom is the bill.
type Reconciliation struct {
	// Unrecorded are projects the cloud holds that bear infrafactory's
	// stamp and that no record explains. The expensive case: something is
	// running and nothing is going to reap it.
	Unrecorded []UnrecordedProject

	// Vanished are records naming projects the API says do not exist.
	// Harmless to the bill, and not harmless: they make `live ls` a lie,
	// and a teardown against one can only fail.
	Vanished []Deployment

	// Accounted is how many records matched a project that exists. Worth
	// reporting rather than implying, because "0 unrecorded" out of zero
	// projects examined and out of forty are different results and read
	// identically.
	Accounted int
}

// UnrecordedProject is a stamped project with no record behind it.
type UnrecordedProject struct {
	ProjectID string
	Name      string
}

// StampedProject is one project the cloud holds, narrowed to what
// reconciliation needs. Declared here rather than importing the harness
// so this package keeps its deliberately small dependency set --
// the same reason PromotionRule takes an injected Normalize.
type StampedProject struct {
	ID   string
	Name string

	// Ours is the CALLER's verdict, taken with the same stamp that guards
	// teardown. Passing the verdict rather than the description keeps one
	// definition of "infrafactory created this" in the codebase, instead
	// of one here and another in the guard.
	Ours bool
}

// Reconcile compares the cloud's projects against the store's records.
//
// Two rules, and each is a way this could be wrong rather than merely
// incomplete:
//
//   - **A project without the stamp is never considered**, in either
//     direction. infrafactory does not reason about projects it did not
//     create, and an unstamped project appearing in this report would
//     invite someone to delete it.
//   - **A released deployment still accounts for its project.** Teardown
//     records the release but the project can outlive it -- ADR-0024's
//     unreclaimable case is exactly that. Ignoring released records would
//     report those projects as unrecorded, sending an operator to
//     investigate something the store already explains.
func Reconcile(projects []StampedProject, deployments []Deployment) Reconciliation {
	known := map[string]bool{}
	for _, d := range deployments {
		if d.ProjectID != "" {
			known[d.ProjectID] = true
		}
	}

	live := map[string]bool{}
	for _, p := range projects {
		live[p.ID] = true
	}

	out := Reconciliation{}
	for _, p := range projects {
		if !p.Ours || known[p.ID] {
			continue
		}
		out.Unrecorded = append(out.Unrecorded, UnrecordedProject{ProjectID: p.ID, Name: p.Name})
	}
	sort.Slice(out.Unrecorded, func(i, j int) bool {
		return out.Unrecorded[i].ProjectID < out.Unrecorded[j].ProjectID
	})

	for _, d := range deployments {
		if d.ProjectID == "" {
			// A record with no project id cannot be reconciled either
			// way. ADR-0024 already reports it as reapable-but-damaged,
			// so it is not this command's to re-report.
			continue
		}
		if live[d.ProjectID] {
			out.Accounted++
			continue
		}
		out.Vanished = append(out.Vanished, d)
	}
	sort.Slice(out.Vanished, func(i, j int) bool { return out.Vanished[i].ID < out.Vanished[j].ID })

	return out
}

// Clean reports whether the store and the cloud agree.
func (r Reconciliation) Clean() bool {
	return len(r.Unrecorded) == 0 && len(r.Vanished) == 0
}
