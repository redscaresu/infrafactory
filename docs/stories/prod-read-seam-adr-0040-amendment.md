---
kind: docs
status: ready
epic: prod-read-seam
depends_on: [prod-read-seam-reader-policy, aws-hold-for-read-adr-0040]
touches: [docs/decisions/0040-aws-whole-scope-ownership.md]
---

# ADR-0040 amended: the seam reads beyond the Layer 3 scope, and only reads

Split out because aws-hold-for-read also amends ADR-0040. This story waits for aws-hold-for-read-adr-0040, docs only, so the two edits are sequential and this one does not wait for the hold's paid run.

Append a dated amendment: the read seam (ADR-0041, the reader table and its wall) reads accounts beyond the Layer 3 scope, and only reads; rule 10 applies to every reader list; the rule 8 sweep table is untouched.

**Done when:**
- The diff of ADR-0040 shows only added lines, after aws-hold-for-read-adr-0040's amendment, which is left unchanged.
- The amendment names ADR-0041 and reader-policy.json.
- TestADR0040NamesEverySweepRowAndPrecedencePair and TestADR0040RecordsTheHold pass unchanged; `make doc-hygiene` passes.
