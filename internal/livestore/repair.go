package livestore

import "time"

// Repair is an upgrade that DEMONSTRABLY fixed something (S156d).
//
// # Why this type exists rather than "the deployment was upgraded"
//
// S155b keeps the previous configuration in `.infrafactory-previous/`
// precisely so a live failure can reach `ExtractFixPitfall`, which turns
// a before/after HCL pair into a PRESCRIPTIVE rule -- one that says what
// to do, not merely what went wrong. That is the strongest class of entry
// the corpus holds, and S156c can only produce the weakest.
//
// The plan's stated risk was that an upgrade is a diff between two
// configurations that BOTH applied successfully, which is not obviously
// the same shape as "this failed, then this passed".
//
// The risk is real and the resolution is that **the apply is not the
// discriminator; the observations are.** An upgrade qualifies only when
// the service was observed failing BEFORE it and observed healthy AFTER.
// That is exactly "this failed, then this passed", measured against the
// running service rather than against terraform -- which is the whole
// point of live observation, since terraform reported success both times.
//
// An upgrade with no failure before it fixed nothing. An upgrade still
// failing after it fixed nothing. Neither is a lesson, and writing either
// one would teach a remedy that was never shown to work.
type Repair struct {
	Deployment Deployment

	// Detail is the failure the upgrade cleared, as observed before it.
	Detail string

	// Example is one raw pre-upgrade detail, for a human.
	Example string

	// ObservationsBefore is how many consecutive probes reported THIS
	// failure immediately before the upgrade -- not the size of the
	// pre-upgrade history.
	//
	// A rule that says "3 probes reported it" when one did, and two
	// earlier probes were healthy, overstates its own evidence. The
	// corpus is read as guidance, so an inflated confidence is a small
	// lie that survives.
	ObservationsBefore int

	// ObservationsAfter is how many probes confirmed it gone.
	ObservationsAfter int
}

// MinHealthyAfterUpgrade is how many consecutive healthy observations
// must follow an upgrade before it counts as a repair.
//
// One is not enough: a service that is briefly up during a restart, or
// probed once in a window it happens to survive, would promote a remedy
// on the strength of a single lucky sample. This is the same reasoning as
// PromotionRule.ConsecutiveProbes, and the same admission -- the number is
// a guess, held here so it can be raised when there is evidence to tune it
// against.
const MinHealthyAfterUpgrade = 2

// Repairs returns the upgrades that demonstrably fixed something.
//
// normalize collapses cosmetic variation in failure details, as in
// PromotionCandidates; nil means identity.
func Repairs(deployments []Deployment, normalize func(string) string) []Repair {
	if normalize == nil {
		normalize = func(s string) string { return s }
	}

	var out []Repair
	for _, d := range deployments {
		if d.UpgradedAt.IsZero() || d.WorkDir == "" {
			continue
		}

		if !d.UpgradeSucceeded {
			// The apply ran but did not complete, so the running
			// infrastructure is a mixture of the two configurations.
			// Diffing them describes a change that was never fully
			// made, and pairing that with "healthy now" would credit
			// the recovery to HCL that was never applied.
			continue
		}
		if d.UpgradeStartedAt.IsZero() {
			// Without the start of the apply there is no way to tell the
			// upgrade's OWN downtime from the failure it was meant to
			// fix: `live observe` can run during an apply that takes
			// minutes, and those probes are stamped before UpgradedAt.
			//
			// Declining is fail-closed and costs only older records.
			// Guessing a boundary would teach remedies for outages the
			// upgrade caused.
			continue
		}

		before, after := splitAtUpgrade(d)
		if len(after) < MinHealthyAfterUpgrade {
			// Not yet enough evidence. Deliberately not an error and
			// not a rejection -- observing it more may still qualify
			// it, and saying "no" now would be as wrong as saying yes.
			continue
		}
		if !allHealthy(after) {
			continue
		}

		detail, example, run := failureAtUpgrade(before)
		if detail == "" {
			// The service was not failing when it was upgraded. Either
			// it was healthy throughout -- a version bump, a fine thing
			// to have done that teaches no remedy -- or it had already
			// recovered on its own, in which case crediting the new
			// configuration would attach a remedy to a failure it never
			// addressed.
			continue
		}

		out = append(out, Repair{
			Deployment:         d,
			Detail:             normalize(detail),
			Example:            example,
			ObservationsBefore: run,
			ObservationsAfter:  len(after),
		})
	}
	return out
}

// splitAtUpgrade divides a deployment's observations either side of its
// most recent upgrade, DISCARDING those taken during the apply.
//
// Three regions, not two. `[UpgradeStartedAt, UpgradedAt)` is the window
// in which the service is expected to be disrupted, and a probe landing
// there describes neither the old configuration nor the new one -- it
// describes the changeover. Counting such a probe as "before" would make
// the upgrade's own downtime look like the failure it was meant to fix,
// and the corpus would gain a remedy for an outage this tool caused.
//
// Observations AT the finish instant count as after: a probe stamped at
// the moment the upgrade completed describes the new configuration, and
// counting it as evidence of the old failure would attribute the fault to
// the thing that fixed it.
func splitAtUpgrade(d Deployment) (before, after []Observation) {
	for _, o := range d.Observations {
		if o.At.Before(d.UpgradeStartedAt) {
			before = append(before, o)
			continue
		}
		if o.At.Before(d.UpgradedAt) {
			// In flight. Discarded rather than assigned to either side.
			continue
		}
		after = append(after, o)
	}
	return before, after
}

func allHealthy(observations []Observation) bool {
	for _, o := range observations {
		// adverse() rather than !Healthy(): a service answering fine
		// while running a version other than the one deployed is NOT
		// evidence that an upgrade worked. It is close to evidence of
		// the opposite -- the apply reported success and the running
		// service did not move.
		if adverse(o) {
			return false
		}
	}
	return len(observations) > 0
}

// failureAtUpgrade returns the failure the service was exhibiting when it
// was upgraded, or "" if it was not exhibiting one.
//
// The LAST observation before the upgrade, not the last adverse one
// anywhere in the history. A service that broke on Monday, recovered on
// Tuesday, and was upgraded on Wednesday was not fixed by that upgrade --
// it had already recovered, and crediting the new configuration would
// attach a remedy to a failure it never addressed.
//
// This is the mirror of the rule on the other side: `after` must be
// entirely healthy, so `before` must end unhealthy. Anything looser lets
// an unrelated diff inherit somebody else's failure.
// It also returns how many consecutive probes reported that same
// failure, which is the evidence the rule may claim -- the trailing run,
// not the whole history. Two healthy probes followed by one 503 is one
// probe's worth of evidence, not three.
func failureAtUpgrade(before []Observation) (detail, example string, run int) {
	if len(before) == 0 {
		return "", "", 0
	}
	last := before[len(before)-1]
	if !adverse(last) || last.Detail == "" {
		return "", "", 0
	}

	for i := len(before) - 1; i >= 0; i-- {
		o := before[i]
		if !adverse(o) || o.Detail != last.Detail {
			break
		}
		run++
	}
	return last.Detail, last.Detail, run
}

// UpgradedAtOrZero is a small accessor so callers outside this package
// can order events against the upgrade without reaching into the record.
func (r Repair) UpgradedAt() time.Time { return r.Deployment.UpgradedAt }
