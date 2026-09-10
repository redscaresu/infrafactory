package feedback

import "sort"

type FailureSignature struct {
	Check    string
	Resource string
	Detail   string
}

func FailureSignatures(failures []Failure) []FailureSignature {
	seen := make(map[FailureSignature]struct{})
	for _, failure := range failures {
		// Normalize Detail before forming the signature so iterations
		// whose underlying bug is identical (`web_0.private_ip` vs
		// `web[*].private_ip`, different line numbers, "Did you mean"
		// suffix) collide into one signature. Without this the LLM's
		// cosmetic mutations across iterations make every signature
		// unique and DetectOscillation / IsStuck silently miss
		// recurring problems. The original Failure.Detail is
		// untouched — only this view is normalized.
		sig := FailureSignature{
			Check:    failure.Check,
			Resource: failure.Resource,
			Detail:   NormalizeDetail(failure.Detail),
		}
		seen[sig] = struct{}{}
	}

	signatures := make([]FailureSignature, 0, len(seen))
	for sig := range seen {
		signatures = append(signatures, sig)
	}
	sort.Slice(signatures, func(i, j int) bool {
		if signatures[i].Check != signatures[j].Check {
			return signatures[i].Check < signatures[j].Check
		}
		if signatures[i].Resource != signatures[j].Resource {
			return signatures[i].Resource < signatures[j].Resource
		}
		return signatures[i].Detail < signatures[j].Detail
	})

	return signatures
}

// IsStuck reports whether this iteration produced nothing the run has not
// already seen -- every current signature has appeared in some EARLIER
// iteration, not merely the immediately previous one.
//
// It used to compare against the previous iteration alone, and that misses
// oscillation, which is the shape a repair loop actually gets stuck in. Run
// 20260910T104418Z went:
//
//	1  gate refusal (instance type)
//	2  apply failure (image)
//	3  gate refusal (instance type)
//	4  destroy failure (private NIC)
//	5  gate refusal (instance type)
//
// No two CONSECUTIVE iterations matched, so the check never fired and the run
// spent its whole budget -- and iterations 2 and 4 were real applies against
// real Scaleway. Alternating between two known failures is not progress; it is
// the classic form of not making any.
//
// Deliberately eager. A signature the run has seen and returned to means it is
// going in circles, and at Layer 3 stopping one iteration early costs a
// regeneration while continuing costs an apply and a destroy. When the two
// errors are priced that differently, the cheap one is the one to make.
func IsStuck(history []FailureSignature, current []Failure) bool {
	currentSigs := FailureSignatures(current)
	if len(currentSigs) == 0 {
		return false
	}

	seen := make(map[FailureSignature]struct{}, len(history))
	for _, sig := range history {
		seen[sig] = struct{}{}
	}

	for _, sig := range currentSigs {
		if _, ok := seen[sig]; !ok {
			return false
		}
	}

	return true
}
