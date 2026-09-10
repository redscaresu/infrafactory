package generator

import (
	"strings"
)

// Layer3GatePrefix marks a refusal produced by the Layer 3 HCL preflight.
// Everything after it is `<file>: <message>`.
const Layer3GatePrefix = "layer 3 refuses this configuration: "

// Layer3AllowlistPrefix marks the OTHER Layer 3 refusal, from the
// resource-type allowlist. It has no `<file>: ` part -- the message names
// the types directly -- and it is the refusal the generator is most likely
// to hit, because reaching for a resource type nobody budgeted for is the
// easiest mistake to make. Matching only the shape-gate prefix silently
// dropped this whole class.
const Layer3AllowlistPrefix = "layer 3 refuses to apply resource type(s) "

// ExtractGatePitfall turns a gate or policy REFUSAL into a prescriptive
// pitfall.
//
// # Why this class deserves its own extractor
//
// The learning loop only ever learned from provider stderr, via
// ExtractDescriptivePitfall. That is the worst available source: it is a
// symptom, written for a human debugging one apply, and it says nothing about
// what to write instead. It is why the corpus fills with `descriptive` dumps
// and every durable rule ends up hand-written.
//
// Gate and policy refusals are the opposite, and they were being thrown away.
// They are machine-generated, exact, and ALREADY PRESCRIPTIVE -- the gate names
// the permitted values, and `vpc_required` names the block to add. They cost
// nothing to produce, they fire before anything is created, and until now they
// died with the run.
//
// The cost of discarding them, measured: run 20260910T104418Z proposed a
// refused instance type in iterations 1, 3 and 5 -- told the permitted set each
// time, and starting the next run with nothing remembered at all.
//
// # Why the refusal text is used verbatim as the rule
//
// Because it is already the right sentence, and because paraphrasing is how
// this goes wrong. Two hand-written pitfalls added on 2026-09-09 were both
// retracted inside a day: one steered the generator to an image whose apply
// fails, the other promised a teardown fix that did not work. A refusal cannot
// have that failure mode -- it is emitted by the code that does the refusing,
// so it cannot drift from what is actually enforced.
//
// Returns nil for anything that is not a refusal. Provider errors keep going
// to ExtractDescriptivePitfall.
func ExtractGatePitfall(failureDetail, scenarioName string) *LearnedPitfall {
	rule := gateRefusalMessage(failureDetail)
	if rule == "" {
		return nil
	}

	resource := ExtractResourceFromDetail(rule)
	if resource == "" {
		// Keyed by resource type, and a rule filed under the wrong type is
		// worse than one not filed at all -- it is offered to the generator
		// whenever that type appears. Refusals that name no type (a bad
		// provider block, a forbidden top-level block) are left to the
		// stage summary, which already reports them.
		return nil
	}

	return &LearnedPitfall{
		Resource:       resource,
		Rule:           rule,
		Source:         "fix",
		DiscoveredFrom: scenarioName,
	}
}

// gateRefusalMessage returns the prescriptive half of a refusal, or "".
func gateRefusalMessage(detail string) string {
	prefix := Layer3GatePrefix
	idx := strings.Index(detail, prefix)
	if idx < 0 {
		// The allowlist refusal keeps its prefix in the rule, because the
		// sentence does not stand up without it: "scaleway_k8s_cluster:
		// not in ..." reads as a fact about the type rather than a
		// decision this repository made about cost.
		if at := strings.Index(detail, Layer3AllowlistPrefix); at >= 0 {
			rest := strings.TrimSpace(detail[at:])
			if cut := strings.Index(rest, " | stderr:"); cut > 0 {
				rest = strings.TrimSpace(rest[:cut])
			}
			// The refusal is plural when several types were rejected
			// ("scaleway_k8s_cluster, scaleway_rdb_instance: not in
			// ..."). A pitfall has ONE resource key, so this would file
			// a rule naming both under whichever came first -- offered
			// to the generator whenever that type appears, talking about
			// another. Skipped rather than mis-filed, on the same rule
			// as refusals that name no type at all: the stage summary
			// already reports every one of them, and the next run will
			// hit them one at a time as they are fixed.
			if names, _, ok := strings.Cut(rest[len(Layer3AllowlistPrefix):], ":"); ok && strings.Contains(names, ",") {
				return ""
			}
			return rest
		}
		return ""
	}
	rest := strings.TrimSpace(detail[idx+len(prefix):])

	// A run failure detail is composed as `<err> | stderr: <stderr>`
	// (stderrFailureDetail), and for a gate refusal both halves are the
	// SAME sentence -- so the naive "everything after the prefix" is the
	// message followed by a duplicate of itself. Cut the tail.
	if cut := strings.Index(rest, " | stderr:"); cut > 0 {
		rest = strings.TrimSpace(rest[:cut])
	}

	// The gate reports every problem it found, joined by "; ". Only the
	// first is kept: they are independent refusals about different
	// resources, and concatenating them produces a rule filed under one
	// type that talks about several.
	// A single refusal contains "; " itself -- `sets type to "X"; the gate
	// permits only [...]` -- so the split point is not the first
	// separator, it is the first one FOLLOWED BY a new problem, which the
	// gate always introduces with the file it was found in. Checking only
	// the first separator truncated nothing when the first one was
	// internal, and let a second refusal ride along.
	for offset := 0; ; {
		cut := strings.Index(rest[offset:], "; ")
		if cut < 0 {
			break
		}
		at := offset + cut
		if looksLikeNewProblem(rest[at+2:]) {
			rest = rest[:at]
			break
		}
		offset = at + 2
	}

	// Strip the leading "<file>.tf: ". The filename is where it was found,
	// not what to do about it, and a pitfall carrying it would age badly
	// the moment the generator renames a file.
	if cut := strings.Index(rest, ".tf: "); cut > 0 && !strings.Contains(rest[:cut], " ") {
		rest = strings.TrimSpace(rest[cut+len(".tf: "):])
	}
	return rest
}

// looksLikeNewProblem reports whether a segment begins a fresh refusal,
// which the gate always introduces with the file it was found in.
func looksLikeNewProblem(segment string) bool {
	cut := strings.Index(segment, ": ")
	if cut <= 0 {
		return false
	}
	head := segment[:cut]
	return strings.HasSuffix(head, ".tf") && !strings.Contains(head, " ")
}
