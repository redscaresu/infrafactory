# ADR-0009: Incremental Deployment Model

## Status
Accepted

## Context
Infrastructure grows incrementally. A project starts with a web server, adds a database weeks later, then Redis months after that. The current factory treats every run as a clean slate — `mockway reset` wipes all state, `.tfstate` is deleted, and the agent generates HCL against an empty cloud account every time.

This doesn't test the real deployment scenario: applying new infrastructure alongside existing resources. When a user adds a database to an existing web server project, `tofu plan` should see the existing web server in state and only plan the database creation. Mockway should already contain the web server resources so the topology evaluator can validate cross-resource connectivity (e.g., database on the same VPC as the web server).

Alternatives considered:
1. **Separate composable scenarios** — write one YAML per resource group, compose them at apply time. Rejected: creates hard dependency/ownership problems (which scenario owns the VPC?) and doesn't match how Terraform state works (one state file per root module).
2. **Incremental generation** — only generate HCL for new/changed resources. Rejected: requires the factory to understand diffs, adds complexity. Regenerating everything and letting `tofu plan` diff is simpler and already how IaC works.
3. **Regression-aware failure handling** — distinguish "broke existing resource" from "new resource failed." Rejected: adds complexity for a case that rarely occurs. The feedback loop handles all failures uniformly.

## Decision
Support incremental deployment through a single evolving scenario model:

1. **Single scenario YAML grows over time** — users add resource blocks as their project evolves. No composition, no layering.
2. **Regenerate all HCL every run** — the factory is stateless. OpenTofu handles incremental diffs via its state file.
3. **Persist mockway state between runs (via `--no-destroy`)** — when using `--no-destroy`, mockway state and `.tfstate` persist after convergence. Without `--no-destroy`, Layer 4 destruction wipes everything. `--no-destroy` is required for iterative development workflows.
4. **Snapshot/restore for feedback iterations** — at run start, snapshot mockway state as a baseline. Between feedback iterations within a run, restore to baseline (not full reset). Each iteration tests "new HCL applied on top of existing infrastructure."
5. **Auto-detect incremental mode** — if mockway has resources, `.tfstate` exists in the output dir, and a previous successful run exists in the run store → incremental. If any is missing → clean. `--clean` flag forces clean mode.
6. **Single-scenario workflow** — mockway is a single shared instance. Incremental runs assume one scenario at a time. Concurrent runs of different scenarios against the same mockway are not supported.

New Mockway endpoints: `POST /mock/snapshot`, `POST /mock/restore`.

New CLI flags: `--clean` (force fresh start), `--no-destroy` (skip Layer 4, preserve state for next run). `--no-destroy` is required for iterative development — without it, destruction wipes all state after convergence and the next run starts clean regardless.

## Consequences
**Benefits**:
- Tests the real deployment path: incremental applies against existing infrastructure.
- Simple mental model: one scenario file per project, grows organically.
- No factory-side diff logic — OpenTofu already does this.
- `--clean` provides an escape hatch for when users want a fresh start.

**Tradeoffs**:
- Mockway needs snapshot/restore capability (new SQLite copy/swap logic).
- Run store needs to track incremental vs clean runs and link to previous runs.
- Destruction verification (Layer 4) destroys ALL resources including baseline by default. `--no-destroy` skips Layer 4 to preserve state for incremental workflows. Without `--no-destroy`, the "state persistence between runs" aspect of this ADR is effectively disabled because destruction wipes everything.
- Mockway is a single shared instance — concurrent runs of different scenarios against the same mockway are not supported. One scenario at a time.

**Migration**:
- Existing training/regression scenarios continue to work unchanged (auto-detected as clean runs since no prior state exists in CI).
- No schema changes required.
- Mockway snapshot/restore is additive (existing `/mock/reset` unchanged).
