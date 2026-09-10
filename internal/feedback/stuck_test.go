package feedback

import "testing"

func TestIsStuck(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		previous []Failure
		current  []Failure
		expected bool
	}{
		{
			name: "equal signatures is stuck",
			previous: []Failure{
				{Check: "policy", Resource: "db.main"},
			},
			current: []Failure{
				{Check: "policy", Resource: "db.main"},
			},
			expected: true,
		},
		{
			name: "subset signatures is stuck",
			previous: []Failure{
				{Check: "policy", Resource: "db.main"},
				{Check: "connectivity", Resource: "lb.main"},
			},
			current: []Failure{
				{Check: "policy", Resource: "db.main"},
			},
			expected: true,
		},
		{
			name: "new signature is not stuck",
			previous: []Failure{
				{Check: "policy", Resource: "db.main"},
			},
			current: []Failure{
				{Check: "policy", Resource: "db.main"},
				{Check: "connectivity", Resource: "lb.main"},
			},
			expected: false,
		},
		{
			name: "empty current failures is not stuck",
			previous: []Failure{
				{Check: "policy", Resource: "db.main"},
			},
			current:  []Failure{},
			expected: false,
		},
		{
			name: "same check and resource but different detail is not stuck",
			previous: []Failure{
				{Check: "validate", Detail: "Error: Reference to undeclared resource on compute.tf"},
			},
			current: []Failure{
				{Check: "validate", Detail: "Error: Unsupported attribute on loadbalancer.tf"},
			},
			expected: false,
		},
		{
			name: "same check resource and detail is stuck",
			previous: []Failure{
				{Check: "validate", Detail: "Error: Reference to undeclared resource on compute.tf"},
			},
			current: []Failure{
				{Check: "validate", Detail: "Error: Reference to undeclared resource on compute.tf"},
			},
			expected: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsStuck(FailureSignatures(tc.previous), tc.current); got != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestFailureSignaturesDeduplicatesAndSorts(t *testing.T) {
	t.Parallel()

	sigs := FailureSignatures([]Failure{
		{Check: "policy", Resource: "db.main"},
		{Check: "connectivity", Resource: "lb.main"},
		{Check: "policy", Resource: "db.main"},
	})

	if len(sigs) != 2 {
		t.Fatalf("expected 2 unique signatures, got %d", len(sigs))
	}
	if sigs[0].Check != "connectivity" || sigs[1].Check != "policy" {
		t.Fatalf("expected deterministic sort order, got %+v", sigs)
	}
}

// The case a per-iteration comparison cannot see, and the one that cost
// two real applies on 2026-09-10. Iterations alternate between two known
// failures: nothing repeats CONSECUTIVELY, and the run is plainly going
// in circles.
func TestIsStuckCatchesOscillationAcrossNonAdjacentIterations(t *testing.T) {
	t.Parallel()

	gateRefusal := []Failure{{Check: "generate", Resource: "compute.tf", Detail: "type PLAY2-PICO not permitted"}}
	applyFailure := []Failure{{Check: "apply", Resource: "compute.tf", Detail: "could not get image"}}

	var history []FailureSignature

	// Iteration 1: new.
	if IsStuck(history, gateRefusal) {
		t.Fatal("the first failure of a run cannot be stuck")
	}
	history = append(history, FailureSignatures(gateRefusal)...)

	// Iteration 2: also new -- a different failure is progress of a kind.
	if IsStuck(history, applyFailure) {
		t.Fatal("a failure never seen before is not stuck")
	}
	history = append(history, FailureSignatures(applyFailure)...)

	// Iteration 3: back to iteration 1's failure. Not adjacent to it, and
	// the old previous-only comparison returned false here -- which is
	// how the run reached iteration 5.
	if !IsStuck(history, gateRefusal) {
		t.Fatal("returning to an earlier failure is going in circles, and must be reported as stuck")
	}
}
