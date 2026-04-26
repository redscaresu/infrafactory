package feedback

import (
	"reflect"
	"testing"
)

func TestDetectOscillationReturnsNilWhenHistoryTooShort(t *testing.T) {
	for _, history := range [][]IterationResult{
		nil,
		{},
		{{Iteration: 1, Failures: []Failure{{Check: "a"}}}},
		{
			{Iteration: 1, Failures: []Failure{{Check: "a"}}},
			{Iteration: 2, Failures: []Failure{{Check: "a"}}},
		},
	} {
		if got := DetectOscillation(history); got != nil {
			t.Errorf("DetectOscillation(%v) = %v, want nil", history, got)
		}
	}
}

func TestDetectOscillationDetectsSimpleOscillation(t *testing.T) {
	history := []IterationResult{
		{Iteration: 1, Failures: []Failure{{Check: "policy", Resource: "rdb", Detail: "encryption_at_rest"}}},
		{Iteration: 2, Failures: []Failure{{Check: "plan", Resource: "instance", Detail: "missing private_nic"}}},
		{Iteration: 3, Failures: []Failure{{Check: "policy", Resource: "rdb", Detail: "encryption_at_rest"}}},
	}

	got := DetectOscillation(history)
	want := []FailureSignature{
		{Check: "policy", Resource: "rdb", Detail: "encryption_at_rest"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDetectOscillationReturnsEmptyForLinearFailure(t *testing.T) {
	history := []IterationResult{
		{Iteration: 1, Failures: []Failure{{Check: "a", Detail: "x"}}},
		{Iteration: 2, Failures: []Failure{{Check: "a", Detail: "x"}}},
		{Iteration: 3, Failures: []Failure{{Check: "a", Detail: "x"}}},
	}

	if got := DetectOscillation(history); len(got) != 0 {
		t.Errorf("expected no oscillation for sustained failure, got %v", got)
	}
}

func TestDetectOscillationReturnsEmptyForNewFailureAtEnd(t *testing.T) {
	// Pattern: A, B, C — no signature appears, disappears, then reappears.
	history := []IterationResult{
		{Iteration: 1, Failures: []Failure{{Check: "a"}}},
		{Iteration: 2, Failures: []Failure{{Check: "b"}}},
		{Iteration: 3, Failures: []Failure{{Check: "c"}}},
	}

	if got := DetectOscillation(history); len(got) != 0 {
		t.Errorf("expected no oscillation for distinct failures, got %v", got)
	}
}

func TestDetectOscillationDetectsMultipleOscillatingSignatures(t *testing.T) {
	a := Failure{Check: "policy", Resource: "instance", Detail: "no_public_endpoints"}
	b := Failure{Check: "plan", Resource: "rdb", Detail: "volume_type"}
	other := Failure{Check: "plan", Resource: "k8s", Detail: "version"}

	history := []IterationResult{
		{Iteration: 1, Failures: []Failure{a, b}},
		{Iteration: 2, Failures: []Failure{other}},
		{Iteration: 3, Failures: []Failure{a, b}},
	}

	got := DetectOscillation(history)
	want := []FailureSignature{
		{Check: "plan", Resource: "rdb", Detail: "volume_type"},
		{Check: "policy", Resource: "instance", Detail: "no_public_endpoints"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestDetectOscillationNonMatchOnTwoIterationGap pins the documented
// "single-iteration absence" contract — a [A, B, B, A] history must NOT
// be flagged as oscillating, because there's no point at which A is
// missing for exactly one iteration. Guards a future "smarter" detector
// from silently widening the contract.
func TestDetectOscillationNonMatchOnTwoIterationGap(t *testing.T) {
	a := Failure{Check: "policy", Detail: "x"}
	b := Failure{Check: "plan", Detail: "y"}

	history := []IterationResult{
		{Iteration: 1, Failures: []Failure{a}},
		{Iteration: 2, Failures: []Failure{b}},
		{Iteration: 3, Failures: []Failure{b}},
		{Iteration: 4, Failures: []Failure{a}},
	}

	if got := DetectOscillation(history); len(got) != 0 {
		t.Errorf("expected no oscillation for two-iteration absence, got %v", got)
	}
}

func TestDetectOscillationDetectsOscillationWithinLongerHistory(t *testing.T) {
	// Pattern: distinct failures for first 3 iterations, then A oscillates
	// across the last 3.
	a := Failure{Check: "policy", Resource: "rdb", Detail: "x"}
	other1 := Failure{Check: "plan", Detail: "p1"}
	other2 := Failure{Check: "plan", Detail: "p2"}

	history := []IterationResult{
		{Iteration: 1, Failures: []Failure{other1}},
		{Iteration: 2, Failures: []Failure{other2}},
		{Iteration: 3, Failures: []Failure{a}},
		{Iteration: 4, Failures: []Failure{other1}},
		{Iteration: 5, Failures: []Failure{a}},
	}

	got := DetectOscillation(history)
	want := []FailureSignature{
		{Check: "policy", Resource: "rdb", Detail: "x"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
