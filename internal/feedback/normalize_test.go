package feedback

import (
	"strings"
	"testing"
)

// TestNormalizeDetail_UnsupportedAttributeKernelCollidesAcrossShapes
// pins the motivating case for the run captured in
// 20260529T095429Z: iter 1 used `web[*].private_ip`, iter 4 used
// `web_0.private_ip` / `web_1.private_ip`, the phrasing of the
// provider error differed ("does not have" vs "no argument, nested
// block, or exported attribute"), and line numbers shifted. Both
// iterations must normalize to the same kernel so DetectRecurringFailures
// flags `private_ip` on `scaleway_instance_server` as a stable
// learning candidate.
func TestNormalizeDetail_UnsupportedAttributeKernelCollidesAcrossShapes(t *testing.T) {
	iter1 := `exit status 1 | stderr: ╷
│ Error: Unsupported attribute
│
│   on loadbalancer.tf line 21, in resource "scaleway_lb_backend" "web":
│   21:   server_ips       = scaleway_instance_server.web[*].private_ip
│
│ This object does not have an attribute named "private_ip".
╵`
	iter4 := `exit status 1 | stderr: ╷
│ Error: Unsupported attribute
│
│   on loadbalancer.tf line 23, in resource "scaleway_lb_backend" "web":
│   23:     scaleway_instance_server.web_0.private_ip,
│   24:     scaleway_instance_server.web_1.private_ip,
│
│ This object has no argument, nested block, or exported attribute named
│ "private_ip". Did you mean "private_ips"?
╵`

	got1 := NormalizeDetail(iter1)
	got4 := NormalizeDetail(iter4)
	if got1 != got4 {
		t.Fatalf("kernel mismatch — iter1=%q iter4=%q (must be equal so recurrence detection fires)", got1, got4)
	}
	if !strings.Contains(got1, "unsupported_attribute") {
		t.Errorf("kernel missing family tag: %q", got1)
	}
	if !strings.Contains(got1, "private_ip") {
		t.Errorf("kernel missing attribute name: %q", got1)
	}
	if !strings.Contains(got1, "scaleway_instance_server") {
		t.Errorf("kernel missing resource: %q", got1)
	}
}

// TestNormalizeDetail_DifferentAttributesProduceDifferentKernels — iter
// 1 referenced `private_ip` and iter 2 referenced `public_ip` in the
// same run. They are DIFFERENT mistakes (each its own pitfall) and
// must NOT collide into one signature.
func TestNormalizeDetail_DifferentAttributesProduceDifferentKernels(t *testing.T) {
	priv := `Error: Unsupported attribute on scaleway_instance_server.web[*].private_ip — attribute named "private_ip"`
	pub := `Error: Unsupported attribute on scaleway_instance_server.web[*].public_ip — attribute named "public_ip"`
	if NormalizeDetail(priv) == NormalizeDetail(pub) {
		t.Fatal("expected different kernels for private_ip vs public_ip")
	}
}

// TestNormalizeDetail_UnsupportedAttribute_AllClouds — kernel
// extraction is cloud-agnostic. AWS and GCP variants of the same
// shape must produce stable kernels.
func TestNormalizeDetail_UnsupportedAttribute_AllClouds(t *testing.T) {
	cases := map[string]string{
		"scaleway": `Error: Unsupported attribute on scaleway_instance_server.web[0].private_ip — attribute named "private_ip"`,
		"google":   `Error: Unsupported attribute on google_compute_instance.web[0].private_ip — attribute named "private_ip"`,
		"aws":      `Error: Unsupported attribute on aws_instance.web[0].private_ip — attribute named "private_ip"`,
	}
	for cloud, detail := range cases {
		k := NormalizeDetail(detail)
		if !strings.Contains(k, "unsupported_attribute") {
			t.Errorf("%s: kernel missing family tag: %q", cloud, k)
		}
		if !strings.Contains(k, "private_ip") {
			t.Errorf("%s: kernel missing attribute: %q", cloud, k)
		}
		expectedResource := map[string]string{
			"scaleway": "scaleway_instance_server",
			"google":   "google_compute_instance",
			"aws":      "aws_instance",
		}[cloud]
		if !strings.Contains(k, expectedResource) {
			t.Errorf("%s: kernel missing resource %q: %q", cloud, expectedResource, k)
		}
	}
}

// TestNormalizeDetail_UnsupportedArgumentKernel — same family, different
// shape (argument on resource block vs attribute access).
func TestNormalizeDetail_UnsupportedArgumentKernel(t *testing.T) {
	a := `Error: Unsupported argument on line 12: "labels" is not expected here for google_container_cluster.main`
	b := `Error: Unsupported argument on line 99: "labels" is not expected here for google_container_cluster.different_name_42`
	if NormalizeDetail(a) != NormalizeDetail(b) {
		t.Fatalf("expected kernels to match across line/instance variation, got %q vs %q", NormalizeDetail(a), NormalizeDetail(b))
	}
}

// TestNormalizeDetail_GenericFallbackStripsNoise — non-kernel
// failures still benefit from line-number / suffix stripping so
// repeats with cosmetic shifts collide.
func TestNormalizeDetail_GenericFallbackStripsNoise(t *testing.T) {
	a := `exit status 1 | stderr: failed to apply on file.tf line 21, resource scaleway_rdb_instance.db_0`
	b := `exit status 1 | stderr: failed to apply on file.tf line 33, resource scaleway_rdb_instance.db_1`
	if NormalizeDetail(a) != NormalizeDetail(b) {
		t.Fatalf("generic-fallback normalization should erase line numbers + index suffixes, got %q vs %q", NormalizeDetail(a), NormalizeDetail(b))
	}
}

// TestNormalizeDetail_EmptyReturnsEmpty — preserves the empty contract.
func TestNormalizeDetail_EmptyReturnsEmpty(t *testing.T) {
	if NormalizeDetail("") != "" {
		t.Fatal("expected empty input to normalize to empty")
	}
}

// TestDetectRecurringFailures_FlagsFourApartPattern pins the exact
// pattern from the failing run: [A, B, C, A, D]. A appears in iters
// 0 and 3 — outside any 3-iteration toggle window — so
// DetectOscillation cannot catch it, but DetectRecurringFailures
// must.
func TestDetectRecurringFailures_FlagsFourApartPattern(t *testing.T) {
	a := Failure{Check: "validate", Detail: `Unsupported attribute on scaleway_instance_server — attribute named "private_ip"`}
	b := Failure{Check: "validate", Detail: `Unsupported attribute on scaleway_instance_server — attribute named "public_ip"`}
	c := Failure{Check: "policy", Resource: "scaleway_instance_server", Detail: "vpc required"}
	d := Failure{Check: "test", Detail: "connectivity broken"}

	history := []IterationResult{
		{Iteration: 1, Failures: []Failure{a}},
		{Iteration: 2, Failures: []Failure{b}},
		{Iteration: 3, Failures: []Failure{c}},
		{Iteration: 4, Failures: []Failure{a}},
		{Iteration: 5, Failures: []Failure{d}},
	}

	// Sanity check: DetectOscillation cannot see this pattern.
	if osc := DetectOscillation(history); len(osc) != 0 {
		t.Fatalf("precondition: this pattern should NOT trip DetectOscillation, got %v", osc)
	}
	recurring := DetectRecurringFailures(history)
	if len(recurring) != 1 {
		t.Fatalf("expected exactly one recurring signature, got %d: %v", len(recurring), recurring)
	}
	if !strings.Contains(recurring[0].Detail, "private_ip") {
		t.Errorf("recurring signature missing attribute name in detail: %+v", recurring[0])
	}
}
