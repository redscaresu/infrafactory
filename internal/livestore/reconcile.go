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

	// Vanished are LIVE records naming projects the API says do not
	// exist. Harmless to the bill, and not harmless: they make `live ls`
	// a lie, and a teardown against one can only fail.
	//
	// Released records are excluded: teardown destroys the project and
	// then marks the record released, so a released record whose project
	// is gone is the success path.
	Vanished []Deployment

	// Released are released records whose project STILL EXISTS.
	//
	// Not a disagreement and not agreement either. `live forget` marks a
	// record released "WITHOUT destroying anything and WITHOUT verifying
	// the account" -- it is the escape hatch for a record teardown
	// cannot act on -- so a surviving project may be an empty shell left
	// by a partial teardown, or it may be a load balancer and an
	// instance still billing with nothing that will ever reap them.
	//
	// Reconcile cannot tell those apart; what it must not do is call
	// either of them "the cloud and the store agree". Counting them as
	// Accounted did exactly that, which is the D6 shape this command
	// exists to catch.
	Released []Deployment

	// Retired is how many released records named a project that is gone
	// -- the success path, and the steady state after every teardown.
	//
	// A count rather than a list because there is nothing to report
	// about them; it exists so `Examined` can say they were looked at.
	// Without it the summary said "0 record(s)" for a store holding
	// one, which reads exactly like an empty or unreadable store.
	Retired int

	// Accounted is how many LIVE records matched a project that exists.
	// Worth
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
// Three rules, and each is a way this could be wrong rather than merely
// incomplete:
//
//   - **A project without the stamp is never considered**, in either
//     direction. infrafactory does not reason about projects it did not
//     create, and an unstamped project appearing in this report would
//     invite someone to delete it.
//   - **A released deployment still explains its project.** Teardown
//     records the release but the project can outlive it -- ADR-0024's
//     unreclaimable case, and `live forget`, are exactly that. Ignoring
//     released records would report those projects as unrecorded,
//     sending an operator to investigate something the store already
//     explains. Explained is not AGREED, though: they are reported as
//     `Released`, because forget destroys nothing and a surviving
//     project may still be billing.
//   - **A released deployment whose project is GONE is the success
//     path**, not a disagreement: teardown destroys the project and
//     then marks the record released. Counting it as Vanished left
//     `live reconcile` permanently non-zero after every successful
//     teardown. It is still counted by `Examined`, because a record
//     nobody counts reads as a store with nothing in it.
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
			if d.State == StateReleased {
				// Explained, so never Unrecorded -- but not agreement.
				// See the field comment: `live forget` releases without
				// destroying, so this project may still be billing.
				out.Released = append(out.Released, d)
				continue
			}
			out.Accounted++
			continue
		}
		if d.State == StateReleased {
			out.Retired++
			// Gone, and SUPPOSED to be gone: teardown destroys the
			// project and then marks the record released, so this is
			// the success path rather than a disagreement.
			//
			// Without it every successful teardown left `live
			// reconcile` permanently red, reporting "the record
			// outlived its infrastructure" about the one case where
			// that is exactly what should have happened. Found by
			// running the S156e validation deploy end to end; no unit
			// test reached it, because none of them tore one down
			// first.
			//
			// Ordered AFTER the accounted check on purpose: skipping
			// released records outright would drop the unreclaimable
			// case above, which is the expensive one.
			continue
		}
		out.Vanished = append(out.Vanished, d)
	}
	sort.Slice(out.Vanished, func(i, j int) bool { return out.Vanished[i].ID < out.Vanished[j].ID })
	sort.Slice(out.Released, func(i, j int) bool { return out.Released[i].ID < out.Released[j].ID })

	return out
}

// Clean reports whether the store and the cloud agree.
func (r Reconciliation) Clean() bool {
	// `Released` is deliberately NOT a disagreement. `live forget` is a
	// deliberate operator act that already prints what it is doing, and
	// failing every later reconcile because somebody used it would be
	// the permanent-red defect this command just had. It is REPORTED
	// instead -- visible in the summary, silent in the exit code.
	return len(r.Unrecorded) == 0 && len(r.Vanished) == 0
}

// Examined is how many records were looked at, whatever became of them.
//
// Summed here rather than at the call site, because the call site
// summed `Accounted + Vanished` and a third bucket then made records
// invisible: after any successful teardown the summary said "0 live
// record(s)" for a store that held one, which is the false-signal shape
// this command's own docstring exists to prevent.
func (r Reconciliation) Examined() int {
	return r.Accounted + len(r.Vanished) + len(r.Released) + r.Retired
}
