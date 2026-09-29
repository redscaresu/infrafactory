# Codex review pass 108 — S159b

One finding, accepted.

## [P2] Closing the browser tab could cancel a destroy mid-flight

The handlers passed `r.Context()` straight through. That context is cancelled
when the client disconnects, so navigating away during a teardown would abort a
destroy that had already removed some resources and not others — leaving the live
record describing neither the old state nor the new one.

The rule is already in this codebase, in `ensureRunProject`: *"The create is NOT
cancellable by the caller's context."* Once an operation begins changing real
infrastructure, the caller going away must not stop it.

Destructive handlers now run on `context.WithoutCancel` plus a 30-minute
backstop — generous rather than tight, because a real destroy-plus-sweep-plus-
project-delete has taken minutes against Scaleway and cutting one short is the
failure this timeout exists to *avoid*, not to cause.

## The first test failed against correct code

It captured the context and asserted on it after `ServeHTTP` returned — by which
time the handler's own `defer cancel()` had fired. It was measuring the cleanup,
not the disconnect.

Worth recording because the instinct on a red test is to change the code, and the
code was right. The test now records `ctx.Err()` *during* the call.

## Two passes, two findings, both the same

Pass 107: teardown required LLM credentials, and `withRuntimeNoGenerator` already
existed with a comment explaining why that is wrong. Pass 108: teardown was
cancellable, and `ensureRunProject` already says why that is wrong.

Both rules live next to the code that follows them — which is exactly where a new
path cannot see them.
