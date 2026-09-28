---
kind: code
status: ready
epic: avoid-pitfall-retirement
depends_on: [avoid-learned-layer]
touches: ["internal/generator/pitfalls_avoid_retire.go", "internal/generator/pitfalls_avoid_retire_test.go", "internal/generator/pitfalls_avoid_ledger_ratchet_test.go", "internal/generator/pitfalls_learn.go", "internal/generator/pitfalls_learn_test.go", "docs/decisions/0034-a-prohibition-is-a-specification.md", "docs/decisions/README.md"]
---

# An avoid rule leaves the corpus only into a ledger that keeps its evidence; a mock recurrence is refused and a real-cloud one re-learned

New internal/generator/pitfalls_avoid_retire.go: the ledger types with read and append (failure behaviour per evidence_contract.ledger), ShapeSHA256, ParseAvoidRule, and RetireAvoidPitfall. RetireAvoidPitfall writes the ledger record first and then removes the entry through the existing atomic temp+rename writer (pitfalls_learn.go:891-935, generalised to marshal any value). It reuses assertCloudName and loadCloudPitfalls (pitfalls_retire.go:165,187) read-only; pitfalls_retire.go is not edited. It refuses and leaves both files byte-identical when any of these hold: the rule does not parse; the outcome is not contradicted with both exits 0; a check field is empty; the shape dir's ShapeSHA256 differs; learned_layer is not mock_deploy and no layer_evidence is given; any corpus entry on R naming an attribute is not a mock-learned avoid entry; or the ledger is malformed. AppendPitfall applies the recurrence rule. Ratchet TestRetiredAvoidPitfallsStayRetired. ADR-0034 amendment describing only the controls in avoid-learned-layer and this story.

**Done when:**
- TestShapeSHA256: the same files written in a different order hash equal; a renamed or edited file changes the hash; a subdirectory or non-.tf file is an error.
- TestParseAvoidRule: round-trips buildAvoidRule with one and with two attributes, and parses pitfalls/aws.yaml:8 exactly. The gcp resource-type rule, a mixed attribute+type rule and free text return not-ok.
- TestRetireAvoidPitfall_Refusals: one case per refusal in scope, each leaving pitfalls/<cloud>.yaml and the ledger byte-identical.
- TestRetireAvoidPitfall_MultiAttribute: a two-attribute entry retires whole with both attributes on the record. A shape that sets only one of them is refused.
- TestRetireAvoidPitfall_KeptAndAppendOnly: a kept outcome leaves the corpus byte-identical and appends a kept record. A second retirement leaves the first record byte-identical, and corpus entries before = after + retired records.
- TestAppendPitfall_RetiredAttribute covers each case with the ledger checked. A mock_deploy or unknown-layer candidate naming a retired attribute, as avoid, fix or descriptive text or wording ParseAvoidRule can't parse, is not appended; the error names the check id. A retired attribute plus a new one is refused whole, and the error names the new attribute. A sandbox_deploy candidate is appended after a relearned record. A candidate naming no retired attribute is appended as today.
- TestAppendPitfall_LedgerUnreadable: a malformed ledger still lets the candidate append and returns an error saying the ledger is unreadable. A missing ledger behaves as today, with no error.
- TestRetiredAvoidPitfallsStayRetired runs over the repo's aws/gcp/scaleway files and passes on an empty ledger. Its checker unit test on temp dirs fails on: an avoid, fix or descriptive corpus entry on R naming a retired attribute (snake or camel case) with no later non-mock relearned record; a retired record with a missing field or a non-mock layer and no layer_evidence; a shape sha mismatch; a malformed ledger. It passes a live entry naming the attribute and an entry with a later non-mock relearned record.
- The existing ratchets and AppendPitfall tests pass unchanged. doc-hygiene CI is green with the ADR-0034 amendment and the regenerated `make adr-index` output.
