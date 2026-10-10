# InfraFactory Agent Working Agreement

For AI coding agents. Human contributors should use `CONTRIBUTING.md`.

## Mission
Build `infrafactory`, a Go CLI + SvelteKit UI that generates and validates OpenTofu across **AWS**, **GCP**, **Scaleway**, and **Genesys Cloud CCaaS** with deterministic, testable behavior. Each cloud's scenarios validate against a deterministic HTTP-level mock (fakeaws / fakegcp / mockway / fakegenesys); S3 routes through a small in-repo shim (`cmd/s3router/`) that fans traffic between SeaweedFS (data plane) and fakeaws (`?publicAccessBlock` subresource SeaweedFS doesn't model). See `CONCEPT.md` § "Third-Party Mock Integration".

## Source of Truth
1. `scenario.schema.json`
2. `infrafactory.yaml`
3. `CONCEPT.md` prose

Additional references (map of every live doc: `docs/README.md`; `docs/archive/` is frozen history, skip it when searching):
- Auto-learning loop deep-dive: `docs/auto-learning-loop.md` — single explainer for the mock-server-bug classifier + fix/avoid extractors + diff-pattern templates + ratchets + sweep protocol + worked example
- ADRs: `docs/decisions/*.md`
- Prompts: `prompts/*.md`
- Pitfalls: `pitfalls/{cloud}.yaml` — provider-specific rules loaded at runtime by scenario `cloud` field
- Current state: `STATUS.md`. Open work: one file per item in `docs/stories/`

## Project File Ecosystem

| File | Purpose | When to update |
|---|---|---|
| `docs/hld/*.md` | A design agreed with the user (draft → agreed → superseded); its `## Epics` lists the epics and marks each done | Write with `/hld`, split with `/plan-hld` |
| `STATUS.md` | The entry point: the active HLD and anything waiting on the user. Recent is `git log`, not stored. Under 150 lines, CI-enforced. | Only when Now changes |
| `docs/epics/*.md` | A goal bigger than one PR: **Done when**, out of scope, constraints. Stories join it with `epic:`. | Scope with `/plan-epic <slug>`; the last story's PR deletes it |
| `docs/stories/*.md` | One open item each, `status: ready\|blocked\|later`; a `ready` story is an agent's brief, a `kind: lead` story is the lead's, with the user's part (a decision or a hand step) on a **You:** line. | Open an item by adding a file; the PR that finishes it deletes the file |
| `CONCEPT.md` | Durable architecture, contracts, design decisions | Only for major architecture/design shifts |
| `docs/decisions/*.md` | ADRs for decision-impacting changes | When change crosses ADR trigger threshold (see below) |

## Planning new work

Work flows HLD → epics → stories → the board, run by the swarm-dev plugin (`docs/operations.md` § The swarm):
`/hld` writes an HLD with the user; `/plan-hld <file>` splits an agreed HLD into epics;
`/plan-epic <slug>` scopes an epic into stories; `swarm.sh conduct <epic>` builds its ready
stories. After the HLD, `/swarm` runs these in turn and resumes wherever it stopped. An epic is done when its last story's PR deletes the epic file and marks it done in the
HLD's `## Epics`. One-off work is a single file in `docs/stories/`. A decision or a hand step for
the user is a **You:** line in a `kind: lead` story, so it shows on the board, not in prose.

Retired (kept in git as history, not written any more): arc plans in `docs/archive/plans/`, arc close-outs
in `docs/archive/status/ARCHIVE.md` and `docs/archive/status/STATUS_HISTORY.md`, and slice numbers (`S###`).

ADRs only when crossing the threshold below.

## Fresh Context

When starting a new conversation, follow this checklist:

### 1) Load minimal context
1. `AGENTS.md` (this file)
2. `STATUS.md` — what is in flight, and where open work and history live
3. The active HLD, if `STATUS.md` § Now names one, and the board: `grep -H '^status:' docs/stories/*.md`

On demand only: `README.md`, `CONCEPT.md` (major design context), `docs/decisions/README.md`
(the generated ADR index) and the ADRs it points at.

### 2) Preflight
```bash
git status --short
git branch --show-current
git log -1 --oneline
```
- If unexpected local changes appear, stop and ask the user.
- Take the active HLD from `STATUS.md` § Now, and the queue from the `ready` stories in `docs/stories/`.

### 3) Startup verification
```bash
go test -tags noui ./...    # Use -tags noui until ui/build/ exists
bash scripts/check_all.sh
```
If either fails, restore the repo to a green baseline before starting a new ticket.

### 4) Operational caveats
- Prefer `run` over manual `generate` + `test` — only `run` feeds prior iteration failures into LLM generation.
- Use `http://127.0.0.1:8080` for local Mockway checks (more reliable than `localhost`).
- Port 8080 conflicts are common — check for stale containers before `mock start`.
- Debug iterative behavior from `.infrafactory/runs/<scenario>/<run-id>/iterations/<n>/iteration.json`.
- `output/<scenario>/` is mutable (overwritten each run); immutable snapshots live under `.infrafactory/runs/<scenario>/<run-id>/generated/`.
- No `unset CLAUDECODE` is needed before running infrafactory from a Claude Code session: `claude_adapter.go` strips the parent session's `CLAUDECODE` and `CLAUDE_CODE_*` variables.
- Mock rebuild required after any sibling-mock code change: `pkill -f <mock-bin>; cd ../<mock> && go build && ./<bin> --port <port> &`. For containerised runs use `make mocks-down-containers && make mocks-up-containers`.
- Build tag: `-tags noui` required when `ui/build/` doesn't exist. The `!noui` build requires `ui/build/`.
- Playwright e2e tests live in `ui/e2e/`. `make test` runs Go unit + UI unit + Playwright. The pre-commit hook tests only the Go packages a commit touches (`PRECOMMIT_FULL=1` for everything); CI runs the full suite and blocks merge (ADR-0035).
- Visual baselines under `ui/e2e/visual.spec.ts-snapshots/` render live UI state — adding scenario YAMLs OR completing runs (which add rows to the Runs page) drifts them. Pre-commit hook auto-refreshes when `scenarios/training/*.yaml` changes (M56); for other drift, run `make ui-baseline-update` manually.
- `make run` builds everything and starts the UI at `http://127.0.0.1:4173`.
- `make up` is the one-shot bring-up: mockway + fakegcp + fakeaws + fakegenesys + SeaweedFS + UI in one command. `make down` tears down the mocks (Ctrl-C stops the UI).
- **Sweeps**: mock-server gaps are fixed at source in the sibling mock, never seeded into `pitfalls/*.yaml`; HCL mistakes are left for auto-learning. Full protocol, ratchets and mock-gaps drainage: `docs/auto-learning-loop.md`.
- **Cross-repo cascade commits**: lifecycle-parity work spans infrafactory + a sibling mock. Commit the mock-side change first (it's the dependency), then update infrafactory's e2e test or call sites that depend on the new mock behavior. All four repos use origin/main; push order matters.

## Execution Loop (mandatory)
1. Frame task with `docs/process/TICKET_TEMPLATE.md`.
2. Classify change: `implementation-only` or `decision-impacting`.
3. If `decision-impacting`, create/update ADR (`docs/decisions/NNNN-title.md`).
4. Implement smallest runnable vertical slice.
5. Add/update focused tests.
6. Run `go test ./...` (or report why not possible).
7. Sync docs: delete the story file the PR finishes, add one for work it opens. `STATUS.md` only if Now changes; `CONCEPT.md` for major shifts; `AGENTS.md` only when workflow changes.
8. Run hygiene check: `bash scripts/check_all.sh`.

## Sibling Mock Repos

mockway (`../mockway`, Scaleway, `:8080`), fakegcp (`../fakegcp`, `:8081`), fakeaws
(`../fakeaws`, `:8082`), fakegenesys (`../fakegenesys`, `:8083`), plus SeaweedFS (`:9090`) behind
the in-repo `s3router` (`:9091`). Before changing a mock or anything that dispatches to one, read
`docs/operations.md` § Sibling mocks.

## ADR Trigger Threshold
A change under `internal/cli/`, `cmd/infrafactory/` or `infrafactory.yaml` that crosses none of the lines below carries `ADR: none — <reason>` in its commit message instead of an ADR edit (ADR-0035).

Create/update ADR when change affects:
- public CLI contract/wiring
- cross-package architecture boundaries
- schema semantics (`scenario.schema.json`, `infrafactory.yaml`)
- external dependency strategy (tofu/mockway/opa integration model)
- durable workflow governance

## Writing it down once

Each change is described in one place, and everything else points there
(ADR-0035). Prose is the most expensive thing this workflow produces, and every
copy of a story is another place for it to go stale.

- **The PR body** is the full account: what changed, why, and the evidence.
  The squash commit is its first paragraph.
- **Recent work is `git log`**: the squash-commit title is the line. Open work is a
  file in `docs/stories/`, deleted by the PR that finishes it. No shared list is
  edited per PR, so parallel PRs do not conflict on it.
- **An ADR** only for a decision that crosses the threshold above. Evidence and
  verification runs belong in the PR, not appended to the ADR.
- **Code comments** say what the rule is and why, in a few lines. How it got
  that way belongs to `git log` and the ADR. A claim about another component is
  a test, not a comment (ADR-0028).
- **No counts or measurements in comments or committed docs** unless dated and
  scoped. This file said "51 Playwright tests" long after there were 136.

## Engineering Rules
- Keep command handlers thin; put logic in `internal/*` packages.
- Keep packages cohesive: `internal/cli`, `internal/config`, `internal/scenario`, `internal/generator`, `internal/harness`, `internal/feedback`, `internal/runstore`, `internal/api`.
- `ui/` — SvelteKit frontend (adapter-static, embedded via `go:embed`).
- Use explicit structs and typed errors.
- Keep behavior deterministic and tests hermetic where possible.
- **Assertion convention (Go tests, project-wide)**: default to `github.com/stretchr/testify` for assertions where possible. `assert.Equal` / `require.NoError` / `assert.Contains` over `if x != y { t.Fatalf(...) }`. Use `require` when a failure should stop the test (setup steps, anything whose failure would cause downstream nil-deref / panic). Use `assert` otherwise so multiple failures surface in one run. Don't bare-literal HTTP status codes — `http.StatusNoContent` not `204`. The qualifier "where possible" is real: if an if-fatalf block carries a fundamentally custom error message the assertion library can't express, leave it stdlib — don't force conversions that hurt readability. Same rule lives in every sibling fake's AGENTS.md.

## Lessons
Rules adopted from review findings that kept recurring (`swarm.sh lessons`); each line is tagged
with its kind. Every swarm brief also carries these lines.
- Every new or changed test must be shown to fail: back up the fixed file with cp, break the fix, watch the test fail, restore with cp, and record it in the PR. A reviewer sends back any test that passes with its fix removed. <!-- lesson: vacuous-test -->
- Keep one source of truth for each fact (a holder, a list, a stage name): derive every other view from it rather than copying it, and when a review finds a fact held in two places, remove the copy instead of syncing it. <!-- lesson: state-duplication -->
- When code changes behaviour a doc, runbook step, story **You:** line or PR body describes, change that text in the same PR, and re-read the PR body against the final diff before asking for review. <!-- lesson: docs-misstate-code -->
- Anything that hides secrets or private data (a scrub, a redaction, a filter on what is printed or published) is an allowlist: name what may pass and drop the rest. A denylist of patterns to remove fails open on the first one nobody thought of. Never print a credential file, even redacted; print only its key names. <!-- lesson: denylist -->

## Quality Bar
- `go test ./...` passes for completed stories.
- Stubs must return explicit "not implemented" errors.
- No hidden side effects outside project paths.

## The auto-learning pipeline is load-bearing — never excuse its silence

A run that ends `repair_budget_exhausted` or `stuck` on a failure that is not mock-classified
means the pipeline failed to learn. Treat it as a bug, never as a cold start. Diagnostic protocol:
`docs/auto-learning-loop.md` § When a run learns nothing.

## Layer 3 (real Scaleway)

Layer 3 spends real money. Before any Layer 3 work, read `docs/operations.md` § Layer 3 and
ADR-0023; for AWS, § Layer 3 (AWS) holds the scope's hand setup. Always: it is never wired into
scheduled CI; generated HCL never declares a project or sets `project_id` (ADR-0025); and
`openclaw-prod` is protected only by software guards — do not weaken them.

## Epics, stories and the board

`docs/` is an Obsidian vault: `docs/Board.base` shows stories by status and by epic. Work is
planned HLD → epics → stories → built in parallel: `/hld`, `/plan-hld`, `/plan-epic`, then
`swarm.sh conduct` (`docs/operations.md` § The swarm). Keep links as
markdown links, never `[[wikilinks]]`, so the link test can check them.

## Parallel agents

Every agent runs in its own herdr pane via swarm-dev's `swarm.sh`, which also picks its model and
effort by role. Before scoping or building with agents, read swarm-dev's `docs/method.md` and
`docs/operations.md` § The swarm (infrafactory's own rules).

## Codex review loop (required on every PR)

Every PR gets a codex review loop before merge, including docs-only and "obvious" changes — with
one exception: when codex reports a usage limit, the change goes ahead without it rather than
waiting for the reset. The PR body then says "codex skipped: usage limit", and the lead reviews that
diff before merging.

```bash
codex exec review --base main      # from the PR branch
```

`--base` cannot be combined with a custom prompt, so run the default review
and apply the triage yourself.

**Triage.** Codex produces real correctness findings and endless style
nitpicks; act only on the former.

- **Act on**: correctness bugs, safety regressions (anything that could leave
  real cloud resources uncleaned or make a check report clean when it is not),
  missing test coverage that hides a behavioural assumption, error/stderr
  propagation, auth and wire-shape defects.
- **Push back on**: "could be more idiomatic", renaming suggestions, comment
  rewording, repeat findings on patterns that match the sibling fakes'
  conventions, and tests that would pin behaviour we would accept changing.

Pushing back is a real option — record the finding and the rationale rather
than implementing it to make the reviewer quiet.

**Convergence**: **one pass with no substantive findings.** A pass containing a
substantive finding resets it, so a fix is always followed by at least one more
pass. Only then may the PR merge.

One pass trades away the second look that used to catch a fix introducing a new
defect, so treat your own fix as the most likely thing to be wrong: re-read it
against the defect class before calling the pass. If review keeps finding holes
in the same mechanism, replace the mechanism rather than patching it again.

**Record the loop in the PR body**: how many passes, each finding, and what was
done about it. A declined finding's reasoning goes in the PR body too; add a
`docs/review-passes/<slug>.md` only when it is too long for the body. The numbered
`passN.md` files are history.


## Secrets
- Never commit `.env`, credentials, API keys, or private keys.
- `.gitignore` blocks common secret files (`.env`, `credentials.json`, `*.pem`, `*.key`).
- Pre-commit hook scans staged diffs for secret patterns (`SCW_ACCESS_KEY=`, `OPENROUTER_API_KEY=`, `BEGIN PRIVATE KEY`, etc.).
- If the hook blocks your commit, remove the secret from the file and use environment variables instead.
- Same protections apply to mockway and fakegcp repos.

## Safety
- Never revert/delete unrelated user changes.
- Never use destructive git commands without explicit request.
- If unexpected external changes appear, stop and ask the user.
