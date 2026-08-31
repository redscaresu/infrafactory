# Codex review pass 41 — S166+S167 cutover

`codex exec review --base main` on `s166-cutover` at `dd8a336`.

One finding, accepted — **a regression pass 40 introduced.**

## [P2] Cancellation could race the create it was meant to protect

Pass 40 moved `ensureRunProject` inside `runDeployApply`'s signal guard, which
also handed it the signal-derived context. So a Ctrl-C timed *inside* the
project-create HTTP request would abort the client while the API had already
created the project — leaving one that no marker names, no record mentions, and
no teardown can authorise removing.

Pass 40 traded one window for another: before it, the project could leak between
creation and the guard starting; after it, inside the create itself.

## The fix takes both

Two properties are needed at once, and they are not in tension:

- the guard must be **active** during creation, so a signal is trapped rather
  than killing the process — that is what pass 40 got right;
- the create itself must be **uncancellable**, so it finishes and returns an id
  the caller can act on.

`ensureRunProject` now runs its create on `context.WithoutCancel` with
`runProjectTimeout`, the same shape `releaseRunProject` already uses for the
delete — and for the same reason: **losing the id is worse than the extra
second, because the id is the handle.** The timeout keeps a hung API from making
Ctrl-C feel ignored.

Put inside `ensureRunProject` rather than at the deploy call site, so `test`'s
path — which is also handed a signal-derived context by
`withSandboxInterruptGuard` — gets it too. Same rule this arc has now applied to
the delete, the purge and the deletability guard: a check reached by several
paths belongs where it cannot be forgotten.

Covered by `TestEnsureRunProjectCreatesDespiteACancelledContext`, the mirror of
the existing cancelled-delete test.

## Nothing declined this pass.
