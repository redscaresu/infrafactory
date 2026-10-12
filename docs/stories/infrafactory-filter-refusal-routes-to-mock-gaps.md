---
kind: code
repo: infrafactory
status: blocked
blocked_by: [fakeaws-ec2-filters-never-ignored]
epic: fakeaws-data-sources
depends_on: [fakeaws-ec2-filters-never-ignored]
touches: [internal/generator/pitfalls_learn.go, internal/generator/pitfalls_learn_test.go]
---

# A fakeaws filter refusal reaches the learner as a mock gap, never as a pitfall

After fakeaws-ec2-filters-never-ignored, a filter fakeaws does not model answers 501 NotImplemented
with `<Action>: not modelled by fakeaws: <names>`. The SDK error reads
`StatusCode: 501 ... api error NotImplemented: ...`, which matches none of mockActionableSignals
(internal/generator/pitfalls_learn.go:110-160). So a refused filter in a training run is learned
into pitfalls/aws.yaml: the mock's gap taught to the LLM as an HCL mistake, against the epic's
'never pitfalls/*.yaml'. This already happens at today's 409 (`not supported by fakeaws`), and the
epic widens it to Vpcs, IGWs, Instances, KeyPairs and Images.

Add the signal `not modelled by fakeaws` (lowercase, as the list is matched). It is fakeaws's own
phrase, pinned on that side by MUST[ec2-filter-refusal-names-every-name-sorted], so real AWS never
produces it. Also add today's `not supported by fakeaws`, which the 409 refusals at the current
FAKEAWS_SHA carry until fakeaws-data-sources-pin moves the pin. Do not add `api error notimplemented`: a real AWS NotImplemented is not a mock gap. The
learner then writes the refusal to docs/mock-gaps.md through the existing MockGap path, so it is
visible, not dropped and not learned.

The failure detail in the test is copied verbatim from fakeaws, captured by running the SDK or
`tofu plan` against fakeaws main at the never-ignored merge, and the PR records how. Once
fakeaws-data-source-example lands, its recorded `tofu plan` text must also match; if it differs,
fix forward here. Not a decision path (scripts/check_doc_hygiene.sh:94 names only cmd/infrafactory/
and internal/cli/), and it applies the list's own rule ('Add more here as new mock-gap shapes
surface'), so no ADR trailer.

**Done when:**
- A test feeds the verbatim refusal detail through the learner: it is classed mock-actionable with
  signal `not modelled by fakeaws`, a docs/mock-gaps.md entry is produced, and no pitfall is written.
- A verbatim 409 detail holding `not supported by fakeaws` (from the current pin) is routed the
  same way.
- A detail holding `api error NotImplemented` without the fakeaws phrase stays on the pitfall path.
- Each new test is shown failing by a cp-backup break (each signal removed in turn), recorded in the PR.
