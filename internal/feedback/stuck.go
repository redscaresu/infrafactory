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

func IsStuck(previous, current []Failure) bool {
	currentSigs := FailureSignatures(current)
	if len(currentSigs) == 0 {
		return false
	}

	previousSet := make(map[FailureSignature]struct{})
	for _, sig := range FailureSignatures(previous) {
		previousSet[sig] = struct{}{}
	}

	for _, sig := range currentSigs {
		if _, ok := previousSet[sig]; !ok {
			return false
		}
	}

	return true
}
