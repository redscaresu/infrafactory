---
kind: code
status: ready
epic: avoid-pitfall-retirement
depends_on: []
touches: ["internal/cli/run_command.go", "internal/cli/run_command_test.go", "internal/cli/output_contract.go", "internal/feedback/feedback.go", "internal/generator/pitfalls.go", "internal/generator/pitfalls_learn.go", "internal/generator/pitfalls_learn_test.go", "cmd/pitfall-merge/main_test.go"]
---

# A learned pitfall records the layer whose failure taught it

Add FailureSummary.Origin (json:"-", so the output contract is unchanged); runIteration sets it from the failure's original Layer before rewriting it to "run" (run_command.go:1149-1158). Add feedback.Failure.Origin, copied by toFeedbackFailures (run_command.go:1414). Add LearnedPitfall.LearnedLayer and PitfallEntry.LearnedLayer (yaml learned_layer,omitempty); AppendPitfall writes it. Every AppendPitfall call site in run_command.go (:282,:522,:577,:710,:749) sets it from the failure it learned from. An avoid candidate whose learned_layer differs from every near-duplicate's is appended, not deduped, so a real-cloud occurrence of a mock-learned rule is kept beside it. Layer stays "run", so hasConvergeFailure and stuck signatures are untouched. Tip commit: `ADR: none — records provenance; nothing reads it until avoid-check-ledger`.

**Done when:**
- A run-level test with fake harnesses: an avoid rule learned from a mock_deploy apply failure is written with learned_layer: mock_deploy, and one learned from a sandbox_deploy failure with learned_layer: sandbox_deploy.
- TestAppendPitfall_AvoidLayerDedup: an avoid candidate identical to an existing avoid entry but with a different learned_layer is appended; one with the same layer is still deduped. Non-avoid dedup is unchanged.
- TestCommandOutputGoldenSnapshots, the hasConvergeFailure tests and the stuck-detection tests pass unchanged.
- cmd/pitfall-merge round-trips learned_layer (it uses generator.PitfallEntry, main.go:185,202), proven by a merge test that keeps the field.
- TestPitfallsNoHumanSeeding, TestPitfallsSourceEnum and TestPitfallsLearnedFromDiffSnippetCap pass unchanged.
