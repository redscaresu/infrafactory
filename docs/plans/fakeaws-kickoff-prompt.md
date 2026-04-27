# fakeaws kickoff prompt

Self-contained prompt to drive the S43-S48 build to completion. Paste the
fenced block below into a fresh Claude Code session at
`/Users/ehsanashouri/go/src/github.com/redscaresu/infrafactory`. Designed
to survive context compaction via the BACKLOG.md resume contract.

Pre-flight checklist (operational state, branch protection, codex auth,
provider-version pin, mockway port assignments, etc.) lives in
`/Users/ehsanashouri/go/src/github.com/redscaresu/fakeaws/concepts.md` §
"Pre-flight checklist". Read that section before pasting the prompt.

---

```
You are driving the fakeaws build to completion. Plan and ticket
source-of-truth (read all three first, plus the Pre-flight checklist):

  - /Users/ehsanashouri/go/src/github.com/redscaresu/fakeaws/concepts.md
    (esp. § "Pre-flight checklist (before starting S43-T1)")
  - /Users/ehsanashouri/go/src/github.com/redscaresu/infrafactory/docs/plans/slices-43-48-plan.md
  - /Users/ehsanashouri/go/src/github.com/redscaresu/infrafactory/BACKLOG.md

Also read the user's CLAUDE.md, infrafactory's AGENTS.md, and the auto-memory
at ~/.claude/projects/-Users-ehsanashouri-go-src-github-com-redscaresu-infrafactory/memory/MEMORY.md.

The fakeaws repo at /Users/ehsanashouri/go/src/github.com/redscaresu/fakeaws
exists today only as concepts.md — the very first action of S43-T1 is
`git init` there, then ship the four-file Day-1 invariant in commit 1.

EXECUTION RULES

1. Walk the tickets phase-by-phase, S43 → S44 → S45 → S46 → S47 → S48.
   Inside a phase, follow the dep graph in BACKLOG.md (deps column). Do
   not start a ticket until its deps are `done`. Do not skip phases.

2. Per ticket: (a) read the BACKLOG.md row + matching acceptance criteria
   block in slices-43-48-plan.md; (b) implement; (c) run the per-phase
   exit gates from the plan's "Mandatory gates" section; (d) update the
   row from `todo` to `done`; (e) git commit with message
   `<ticket-id>: <one-line summary>`. Author identity:
   `redscaresu <ukashouri@googlemail.com>`.

3. Phase exit (gate 10) requires 2 consecutive codex `NOTHING_TO_IMPROVE`
   passes scoped to that phase's diff. Use the prompt template in
   concepts.md § "Phase 6 (S48) is dedicated to the codex iteration loop"
   — cite recent commit hashes, summarise changes, ask for BLOCKING /
   SUGGEST / NOTHING_TO_IMPROVE findings with file:line citations.
   Archive each pass under fakeaws/docs/review-passes/passN.md.

4. CODEX vs OPUS dispatch:
   - Codex (`codex exec --skip-git-repo-check --sandbox read-only - < prompt`)
     for: review passes (gate 10), independent second-opinion on tricky
     FK/wire-format design choices, lessons-back-to-mockway/fakegcp audits.
   - Opus (you, in this session) for: actual implementation, file edits,
     running tests, debugging.
   - Codex credit-exhaustion fallback: if `codex exec` returns non-zero
     exit AND stderr contains one of `429 / insufficient_quota /
     unauthorized / quota / credits / rate_limit`, switch to ALL-OPUS
     mode for the rest of the run (spawn a general-purpose Agent with the
     same review prompt and treat its output identically). Single
     transient failure (network, timeout) gets one retry first. Note the
     switch in your next BACKLOG.md update.

5. CI gates are mandatory. Six required jobs: `lint`, `build`, `test`,
   `gitleaks`, `regression-seed-audit`, `coverage-audit`, `coverage`.
   Run local equivalents (`go test ./...`, `gitleaks protect --staged`,
   etc.) before committing. Do not bypass with --no-verify. Do not use
   `t.Skip()` except via the manifest-gated `requireHandlerImplemented`
   helper from S43-T10. For inner-loop dev use `go test ./...`; reserve
   `make test` (which adds Playwright e2e) for end-of-phase gates.

6. NO REGRESSION ON EXISTING CLOUDS: when editing infrafactory files
   (especially `internal/cli/runtime.go`, `internal/harness/destroy.go`,
   `internal/harness/topology_derive.go`, `internal/harness/real_probe.go`,
   `internal/cli/test_command.go`, `internal/cli/validate_command.go`,
   `internal/cli/generate_command.go`, `internal/cli/mockway_client.go`,
   `internal/e2e/helpers.go`), the same files serve mockway+fakegcp.
   `go test ./internal/...` must stay green across all three clouds
   before any ticket touching these files can ship.

7. CROSS-POLLINATION (M40, folded into S48-T4): at every phase exit, if
   the codex review surfaces a class of bug shared by mockway or fakegcp,
   file M-tickets against those repos in BACKLOG.md before starting the
   next phase. Filing != closing — they may legitimately stay todo.

8. PER-PR BUNDLE RULE (S43-T10 acceptance): a service is "landed" when
   one PR ships handler + integration test + working/+misconfigured/+
   updates/ dirs (or documented exemption with reason) + scenarios/
   training/aws-<svc>.yaml with aws_resource_anchors + matrix entry in
   fakeaws/coverage_matrix.yaml + flip in handlers/regression_manifest.go::
   LandedServices. Same PR. Don't split.

9. PITFALLS AUTO-LEARNING: when the LLM repeatedly emits a malformed
   shape during scenario generation, append the rule to pitfalls/aws.yaml
   rather than only fixing the immediate scenario. Same pattern as
   pitfalls/scaleway.yaml.

10. RESUME contract: BACKLOG.md is the persistent state. If interrupted
    (compaction, session restart), the next session reads BACKLOG.md,
    finds the lowest-id `todo` whose deps are all `done`, resumes there.
    Don't keep state outside BACKLOG.md. Commit every BACKLOG.md status
    update — uncommitted state is unreachable from the resumed session.

11. BRANCH STRATEGY: long-lived `fakeaws-build` branch with per-ticket
    commits, opened as one PR per phase (six PRs total: S43..S48).
    Per-ticket PRs would be too noisy.

12. FINAL REVIEW (after S48-T8): run a full-repo codex review pass
    covering fakeaws + the infrafactory integration files. If codex
    credits are exhausted, run the same prompt against an Opus
    general-purpose Agent. Fix any BLOCKING findings; SUGGEST findings
    get filed as M-tickets unless trivial. Repeat until 2 consecutive
    NOTHING_TO_IMPROVE.

13. CLEANUP: archive any /tmp/codex-fakeaws-*-output.txt files from the
    planning loop under fakeaws/docs/review-passes/round-NN-planning.md
    so planning history sits beside implementation history.

14. DO NOT STOP. Per the project CLAUDE.md: "Do not ask for permission
    or confirmation before implementing. Just do it. Use Codex agents for
    help without asking. Only stop if you genuinely need clarification on
    requirements." Acceptable stop conditions: (a) all BACKLOG.md fakeaws
    rows `done` AND final review returned 2x NOTHING_TO_IMPROVE; (b) you
    hit a genuine ambiguity the plan docs don't resolve and codex/opus
    consultation didn't disambiguate (in which case write the question
    into BACKLOG.md as a new `blocked` M-ticket and continue with the
    next unblocked work).

Begin by reading the three plan docs and the Pre-flight checklist, then
the user's CLAUDE.md, infrafactory's AGENTS.md, and MEMORY.md. Then
start S43-T1 (`git init` first). Report progress as a one-line status
after each ticket completes (which ticket, which gate batch passed,
which is next). Save longer narrative for review-pass archives.
```

---

## Maintenance notes for this prompt

- If the planning docs are amended after the build starts, update the section references in this prompt to point at the new line numbers / section names.
- The Pre-flight checklist in concepts.md is the load-bearing operational document; this prompt is just the execution rules. Keep the two in sync.
- If the codex CLI changes its credit-exhaustion stderr signatures, update rule 4 to match.
