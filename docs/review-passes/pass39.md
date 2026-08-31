# Codex review pass 39 — S166+S167 cutover

`codex exec review --base main` on `s166-cutover` at `4ee4a6d`.

One finding, accepted — and it broke the clean streak for a good reason.

## [P2] The Layer 3 prompt contradicted itself

`internal/generator/claude_adapter.go`. The cutover added *"do NOT set
`project_id` on any resource"* and left the next bullet in place: *"ensure
resources that require a project are wired to the bootstrapped project"*. There
is no bootstrapped project any more, so following the second bullet means
emitting the attribute the first forbids — and the shape gate refuses it.

Worth more than its P2: a contradictory prompt does not fail loudly. It spends
repair iterations, on the path the whole tool exists for.

Fixed by deleting the stale bullet and folding what it was really saying ("there
is no project to reference") into the rule above it. The history went into a Go
comment, where it belongs — a prompt should state the rule, not narrate its own
revisions.

## What the sweep for the same class found

Pass 34's lesson was that a defect of this shape rarely lands in one file. The
one codex flagged was not the worst instance.

**`pitfalls/scaleway.yaml` still told the generator the NIC was impossible.**
Injected into every Scaleway generation prompt:

> KNOWN CONFLICT (canary 2026-08-30): the NIC takes no `project_id`, so the
> provider creates it in the default project while the server lives in the run's
> own... Layer 1 requires the NIC; Layer 3 cannot apply it. **Unresolved.**

That is precisely the conflict this cutover was written to end, and the canary
had already applied a NIC-bearing plan cleanly. Left alone it would have told
the model to give up on the thing that now works. Rewritten to state the
resolution *and* to warn against the obvious wrong fix — adding a `project_id`,
which the shape gate refuses.

Three docs carried the same stale claim as live fact and now record it as
history with a pointer to the canary: `docs/NEXT_SESSION.md` (its
"THE BLOCKER — read before planning anything Scaleway + compute" section),
`docs/layer3-coverage.md` (twice), and ADR-0024. `run-owned-project-plan.md` is
marked done.

## Nothing declined this pass.

The streak resets: pass 38 was clean, this one was not, so two more consecutive
clean passes are needed.
