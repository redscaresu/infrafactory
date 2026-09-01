# Codex review pass 107 — S159b

One finding, accepted, and it is a good one because the consequence is the
opposite of the intent.

## [P2] Teardown required LLM credentials to start

`teardownActor` called `buildRuntime` with the default options, which construct
the Claude transport. That construction fails on a missing `claude` binary or an
unreadable prompts directory.

So `--allow-teardown` would refuse to start on a machine with no LLM configured —
**making the recovery capability unavailable in exactly the situation that needs
it**: real infrastructure running, and a machine that was never set up to
generate anything.

The pattern already existed and I did not reach for it. `withRuntimeNoGenerator`
exists for `pitfalls retire`, with a comment saying almost this: *"making corpus
maintenance impossible because the LLM is not configured is a dependency nobody
asked for, and it would bite hardest on exactly the machine doing housekeeping
rather than generation."* The same sentence applies to teardown with a stronger
consequence, because the housekeeping here is billable.

The actor now builds with a refusing generator stub — loud if ever called, absent
as a dependency.

## Worth noting

This is the fourth time in two days a finding has been *"the rule is already
stated somewhere in this codebase, in almost these words, and the new path did
not inherit it."* The rules are written next to the code that follows them, which
is exactly where a new path cannot see them.
