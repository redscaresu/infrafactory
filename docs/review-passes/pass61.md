# Codex review pass 61 — S156a

One finding, accepted.

## [P2] Corpus maintenance required the LLM to be configured

`pitfalls retire` was wired through `cfg.withRuntime`, and `buildRuntime`
constructs the Claude transport by default — which can fail on a missing binary
or an unreadable prompts directory.

This command reads a YAML file and writes it back. Making that impossible because
the agent is unconfigured is a dependency nobody asked for, and it bites hardest
on exactly the machine doing housekeeping rather than generation.

`withRuntimeNoGenerator` keeps the logging and error formatting every other
command has, and substitutes a generator that **refuses** rather than one that
returns empty output: this command must never generate, so a call is a bug that
should say so loudly.

The test patches the harness config to point at `/nonexistent/claude`, and
asserts the patch actually applied — otherwise it would pass by constructing a
working transport and prove nothing.

## Nothing declined this pass.
