---
kind: docs
status: blocked
blocked_by: [prod-read-seam-reader-policy, aws-hold-for-read]
epic: prod-read-seam
depends_on: [prod-read-seam-reader-policy, aws-hold-for-read]
touches: [docs/decisions/0040-aws-whole-scope-ownership.md]
---

# ADR-0040 amended: the seam reads beyond the Layer 3 scope, and only reads

Split out because aws-hold-for-read also amends ADR-0040 (aws-hold-for-read.md:13). This story waits for that one, so the two edits are sequential.

`aws-hold-for-read` is that epic's slug, because the epic is not yet planned. Once it is, re-point blocked_by and depends_on to its ADR-0040 story's slug.

Append a dated amendment: the read seam (ADR-0041, the reader table and its wall) reads accounts beyond the Layer 3 scope, and only reads; rule 10 applies to every reader list; the rule 8 sweep table is untouched.

**Done when:**
- The diff of ADR-0040 shows only added lines, after aws-hold-for-read's amendment.
- The amendment names ADR-0041 and reader-policy.json.
- TestADR0040NamesEverySweepRowAndPrecedencePair passes unchanged; `make doc-hygiene` passes.
