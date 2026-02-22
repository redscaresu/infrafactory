package feedback

import "testing"

func TestHoldoutFeedback(t *testing.T) {
	t.Parallel()

	failures := []Failure{
		{Check: "policy", Detail: "public endpoint detected"},
	}

	blocked := HoldoutFeedback(true, failures)
	if blocked != nil {
		t.Fatalf("expected criteria-only holdout feedback to be blocked, got %+v", blocked)
	}

	allowed := HoldoutFeedback(false, failures)
	if len(allowed) != 1 || allowed[0].Detail != "public endpoint detected" {
		t.Fatalf("expected non-criteria holdout feedback to pass through, got %+v", allowed)
	}
}
