package feedback

import "sort"

type FailureSignature struct {
	Check    string
	Resource string
}

func FailureSignatures(failures []Failure) []FailureSignature {
	seen := make(map[FailureSignature]struct{})
	for _, failure := range failures {
		sig := FailureSignature{
			Check:    failure.Check,
			Resource: failure.Resource,
		}
		seen[sig] = struct{}{}
	}

	signatures := make([]FailureSignature, 0, len(seen))
	for sig := range seen {
		signatures = append(signatures, sig)
	}
	sort.Slice(signatures, func(i, j int) bool {
		if signatures[i].Check == signatures[j].Check {
			return signatures[i].Resource < signatures[j].Resource
		}
		return signatures[i].Check < signatures[j].Check
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
