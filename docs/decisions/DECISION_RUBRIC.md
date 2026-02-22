# Decision-Impact Rubric

Use this rubric before coding. If any answer is "yes", the change is `decision-impacting` and needs an ADR.

## Questions

1. Does this change alter a cross-package boundary or architecture path?
2. Does this change modify a public CLI behavior or contract?
3. Does this change alter `scenario.schema.json` semantics or config semantics (`infrafactory.yaml`)?
4. Does this change affect long-term generator/harness behavior in a way that is costly to reverse?
5. Does this change revise source-of-truth precedence or contributor/agent workflow policy?

## Required actions when "yes"

1. Create/update ADR in `docs/decisions/`.
2. Update `docs/decisions/README.md` index.
3. Update `CONCEPT.md` if the durable decision catalog changed.
4. Update `STATUS.md` with concise summary and next action.
