---
kind: code
status: blocked
blocked_by: [layer3-mock-classifier-layer-gated]
depends_on: [layer3-mock-classifier-layer-gated]
touches: [internal/generator/pitfalls_learn.go, internal/generator/pitfalls_learn_test.go, internal/generator/pitfalls_source_ratchet_test.go, internal/cli/run_command.go, internal/cli/run_command_test.go, docs/auto-learning-loop.md]
risk: high
---

# One ordered routing table for learning, one signal table, and a ratchet that asks the router

From the on-call (Fable) review of #512, which merged a narrowed fix. The structure converged, but the
skip vocabulary and the "mock layer" encodings are hand-kept in several places and keep drifting:
`mockLayers` (pitfalls_learn.go ~227) and the avoid ledger's `"" || mock_deploy` (~966) already
disagree on `static`/`destruction`, and every review round added or removed skip signals.

Replace them with:
1. `generator.RouteFailure(layer, stage, detail, layer3)` — one ordered decision table: a gate refusal
   (gateRefusalMessage != "") → learn; mock-era (IsMockLayer, or no origin with Layer 3 off) →
   IsMockServerBug ? mock gap : learn; a sandbox stage outside the learnable set → skip; a signal of
   class weather or mockOnly → skip; otherwise learn.
2. One signal table `[]signal{text, class}` with class ∈ {mockOnly, mockOrCloud, weather}; delete the
   derived lists. Weather signals are added only from an observed run log (`_learning_skipped`),
   never from a review list.
3. The ratchet asks the router: an entry is forbidden iff
   `RouteFailure(learned_layer, "apply", rule, learned_layer != "") != learn`. Delete
   ForbiddenPitfallSignal and the agreement test.
4. One `IsMockLayer`, used by the router and by the avoid-rule ledger.
5. Every writer (the three learner sites and the orphan path) goes through learnsFrom.

**Done when:**
- RouteFailure is the only place a failure's route is decided; a table test covers each row in order,
  including a Layer 3 gate refusal (learned), a mock-layer mock signal (gap), an aws_ami_resolve stage
  (skipped), a real-probe `i/o timeout` (learned), and a no-Origin mock signal with Layer 3 off (gap).
- The ratchet test fails when an entry the router would not learn is present, with no separate signal list.
- The avoid ledger and the router agree on every layer value (one test over all layers).
- Existing learning tests pass unchanged in meaning; each new test is shown failing by a cp-backup break.
