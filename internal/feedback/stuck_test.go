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
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsStuck(tc.previous, tc.current); got != tc.expected {
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
