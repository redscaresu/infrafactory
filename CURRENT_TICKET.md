# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M32
- title: Harden self-review convergence and stuck-signature specificity for run-loop retries
- status: done
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  The run loop could stall on non-converging retries for two avoidable reasons: self-review phase outputs that corrected only a subset of files could implicitly drop untouched files, and stuck detection used signatures that were too coarse (`check` + `resource` only), causing false-positive `stuck` stops.
- Why does it matter now?
  Recent capture-based diagnosis showed failure details evolving across iterations while the run still stopped as `stuck`, reducing effective repair budget and masking actionable model feedback.

## 2) Scope
- In scope:
  1) Merge self-review `# File:` outputs into the existing generated file set (do not replace the entire set).
  2) Ensure both generator adapters use strict parse behavior when self-review output is noncompliant.
  3) Increase stuck-signature specificity by including failure `detail` in signature comparison.
- Out of scope:
  CLI/schema contract changes, new retry policies, and transport provider redesign.

## 3) Acceptance Criteria
1. Self-review partial outputs update only returned files and retain untouched generated files.
2. Self-review outputs with no `# File:` blocks return deterministic parse failure (no silent fallback behavior).
3. Stuck detection does not fire when `check`/`resource` are equal but failure `detail` differs across iterations.
4. Existing true-positive stuck behavior (identical signatures) remains intact.

## 4) Impacted Areas
- Packages/files changed:
  `internal/generator/claude_adapter.go`,
  `internal/generator/openrouter_adapter.go`,
  `internal/generator/claude_adapter_test.go`,
  `internal/feedback/stuck.go`,
  `internal/feedback/stuck_test.go`,
  `internal/cli/run_command.go`,
  `README.md`,
  `ROADMAP.md`,
  `STATUS.md`,
  `BACKLOG.md`,
  `CURRENT_TICKET.md`.
- External contracts affected (CLI/schema/policy):
  no.

## 5) Test Plan
- Unit tests:
  `go test ./internal/generator ./internal/feedback ./internal/cli`
- Integration checks:
  `go test ./...`
- Manual verification:
  Run with `INFRAFACTORY_CAPTURE_LLM_RAW=1` and verify iteration artifacts show evolving failure details without premature false-positive `stuck` caused only by coarse signature matching.

## 6) Risks and Rollback
- Primary risks:
  More sensitive stuck-signature matching may allow additional retries in some previously early-stopped scenarios.
- Rollback approach:
  Revert stuck-signature detail inclusion and self-review merge behavior to prior semantics.

## 7) Done Definition
- Code and tests complete.
- Required docs updated (`STATUS.md`, `BACKLOG.md`, `CURRENT_TICKET.md`, README/ROADMAP notes).
- Remaining follow-up captured explicitly.

## Progress notes
- Updated self-review post-processing to merge returned files into the prior generated file map for both adapters.
- Removed silent Claude self-review no-file-block fallback and aligned strict parse failure behavior.
- Increased stuck-signature precision by including failure detail in signature generation and feedback mapping.
- Added regression coverage for same-check/same-resource but different-detail non-stuck behavior and same-detail true-positive stuck behavior.
- Verified focused test suites and updated docs/tracking files.

## Blocker (if any)
- blocker: none.
