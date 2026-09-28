---
status: active
hld: 2026-09-27-aws-web-stack
depends_on: [fakeaws-step-one-surfaces]
---

# An `avoid` pitfall whose cause was fixed at the source is retired by the pipeline with recorded evidence, never by hand; the map_public_ip_on_launch rule is the first one retired.

**Done when:**
- A retirement path exists for `source: avoid` rules: today the corpus's only outflow is `infrafactory pitfalls retire` (internal/cli/pitfalls_command.go:34) and RetireStaleLivePitfalls removes only `live` entries with a LastSeen (internal/generator/pitfalls_retire.go:42,58-64), so the map_public_ip_on_launch rule (pitfalls/aws.yaml:6-9, source: avoid, discovered_from aws-eks) can never leave (contradiction 10). The path retires an avoid rule only on evidence that contradicts it: the forbidden attribute on the named resource applied cleanly (apply plus empty plan) against the current mock, with the run or check id recorded; a rule whose failure recurs is kept and stamped. The story chooses the mechanism (an operator-invoked re-validation that replays the rule's `discovered_from` scenario without the rule, or a run-time contradiction hook); LLM cost is operator-invoked, never scheduled (feedback: no recurring LLM-cost CI).
- A unit test proves the path never removes a rule without contradicting evidence, and never touches static, fix or live rules.
- pitfalls/aws.yaml no longer carries the map_public_ip_on_launch rule, removed by that path after fakeaws-step-one-surfaces item 5, with the evidence in the commit; the retirement is reviewed as ADR-0034 asks (a prohibition is a specification).

**Out of scope:** Hand-editing any pitfall; retiring static or fix rules; changing the live-rule retention.

**Constraints:** Never hand-edit pitfalls (sweep protocol; HLD Risks). ADR-0034: the prohibition's justification is what is checked. 1000-byte pitfall cap. A run writes learned pitfalls into the checkout it runs from (docs/operations.md:47-50): run from a throwaway worktree.

**Built by:** agents. Swarm-buildable, wave 2. Small, but it owns a mechanism no other epic has and closes an escape clause.

**Areas:** `internal/generator/pitfalls_retire.go`; `internal/cli/pitfalls_command.go`; `pitfalls/aws.yaml (pipeline-written only)`; `docs/decisions/0034 (retirement note, if the mechanism warrants it)`
