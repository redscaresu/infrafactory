package feedback

import "sort"

// IterationResult captures the failure outcome of a single run iteration
// for oscillation analysis. Successful iterations terminate the run loop
// and do not participate in oscillation detection.
type IterationResult struct {
	Iteration int
	Failures  []Failure
}

// DetectOscillation returns failure signatures that oscillate across the
// iteration history. A signature oscillates when it appears in some
// iteration, disappears in the next, and reappears in a later iteration —
// the canonical sign that the model is alternating between two
// incomplete fixes for the same underlying problem.
//
// Returned signatures are deterministic-sorted by Check/Resource/Detail.
// If history has fewer than 3 iterations, no oscillation is possible and
// the result is nil.
func DetectOscillation(history []IterationResult) []FailureSignature {
	if len(history) < 3 {
		return nil
	}

	sigSets := make([]map[FailureSignature]struct{}, len(history))
	allSigs := make(map[FailureSignature]struct{})
	for i, ir := range history {
		set := make(map[FailureSignature]struct{})
		for _, sig := range FailureSignatures(ir.Failures) {
			set[sig] = struct{}{}
			allSigs[sig] = struct{}{}
		}
		sigSets[i] = set
	}

	oscillating := make(map[FailureSignature]struct{})
	for sig := range allSigs {
		for i := 0; i+2 < len(sigSets); i++ {
			_, atI := sigSets[i][sig]
			_, atI1 := sigSets[i+1][sig]
			_, atI2 := sigSets[i+2][sig]
			if atI && !atI1 && atI2 {
				oscillating[sig] = struct{}{}
				break
			}
		}
	}

	out := make([]FailureSignature, 0, len(oscillating))
	for sig := range oscillating {
		out = append(out, sig)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Check != out[j].Check {
			return out[i].Check < out[j].Check
		}
		if out[i].Resource != out[j].Resource {
			return out[i].Resource < out[j].Resource
		}
		return out[i].Detail < out[j].Detail
	})
	return out
}
