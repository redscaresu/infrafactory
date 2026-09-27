# Reusable Execution Prompt

```text
Autonomous execution: follow docs/process/EXECUTION_PROMPT.md exactly; run the ready stories in docs/stories/ sequentially (one at a time) with full per-ticket loop (minimal vertical slice, focused tests, bash scripts/check_all.sh, delete the finished story's file in the same PR), no confirmations, until none remain; halt only for missing mandatory input, sandbox/permission limits, or decision-impacting CLI/schema/architecture changes; on halt output only: ## Blocker ## What Was Tried ## Needed Input.
```
