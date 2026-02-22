# CURRENT_TICKET

Use this file as the per-session execution stub.

## Ticket
- id: M2
- title: Add CI workflow to run tests on PR/main and build binary artifact on successful main push
- status: done
- classification: implementation-only

## 1) Problem Statement
- What is broken or missing?
  Repository lacked a dedicated CI workflow that enforces test execution on PR and main branch merges/pushes, and did not automatically produce a binary artifact after successful main builds.
- Why does it matter now?
  This enforces baseline quality gates in GitHub and provides a deterministic binary build artifact from mainline.

## 2) Scope
- In scope:
  Add GitHub Actions workflow for CI test execution on PR/main and binary build/upload on successful main push.
- Out of scope:
  Release publishing/signing and multi-arch release matrix.

## 3) Acceptance Criteria
1. CI workflow file is added and valid.
2. PRs and main pushes execute `go test ./...`.
3. Successful main push run emits a binary artifact.

## 4) Impacted Areas
- Packages/files changed:
  `.github/workflows/ci.yml`,
  `STATUS.md`,
  `BACKLOG.md`,
  `CURRENT_TICKET.md`.
- External contracts affected (CLI/schema/policy):
  no.

## 5) Test Plan
- Unit tests:
  `go test ./...`
- Integration checks:
  n/a
- Manual verification:
  workflow syntax and trigger rules reviewed.

## 6) Risks and Rollback
- Primary risks:
  workflow runtime drift (Go/action versions) over time.
- Rollback approach:
  revert `.github/workflows/ci.yml`.

## 7) Done Definition
- CI workflow present and validated locally where applicable.
- Required docs updated (`STATUS.md`, `BACKLOG.md`, `CURRENT_TICKET.md`).
- Remaining follow-up captured explicitly.

## Test plan
- `go test ./...`
- `bash scripts/check_all.sh`

## Progress notes
- Added `.github/workflows/ci.yml`.
- Configured workflow triggers:
  - `pull_request` (opened/synchronize/reopened/ready_for_review): run tests.
  - `push` on `main`: run tests, then build binary artifact.
- Added `build-binary` job gated on successful `test` and `push` to `main`.
- Binary build output:
  - `dist/infrafactory-linux-amd64` (Linux amd64, `CGO_ENABLED=0`).
  - Uploaded as workflow artifact `infrafactory-linux-amd64`.

## Blocker (if any)
- blocker: none.
